package emux

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type session struct {
	ctx    context.Context
	cancel func()

	mut sync.RWMutex

	sess map[uint64]*stream

	decode             *Decode
	encode             *Encode
	writerMut          sync.Mutex
	writerCount        int64
	flushCh            chan struct{}
	stm                io.ReadWriteCloser
	isClose            uint32
	isClear            uint32
	instruction        *Instruction
	lastTime           time.Time
	skipUpdateLastTime uint32
	peerClose          chan struct{}
	onceError          onceError

	Logger      Logger
	BytesPool   BytesPool
	Timeout     time.Duration
	IdleTimeout time.Duration
}

func newSession(ctx context.Context, stm io.ReadWriteCloser, instruction *Instruction) *session {
	ctx, cancel := context.WithCancel(ctx)
	s := &session{
		ctx:         ctx,
		cancel:      cancel,
		sess:        map[uint64]*stream{},
		instruction: instruction,
		stm:         stm,
		flushCh:     make(chan struct{}, 1),
		peerClose:   make(chan struct{}),
		Timeout:     DefaultTimeout,
		IdleTimeout: DefaultIdleTimeout,
	}
	return s
}

func (s *session) IsClear() bool {
	return atomic.LoadUint32(&s.isClear) == 1
}

func (s *session) Clear() {
	if !atomic.CompareAndSwapUint32(&s.isClear, 0, 1) {
		return
	}

	s.cancel()

	done := make(chan struct{})
	go func() {
		err := s.execCmd(s.instruction.Close)
		if err != nil {
			if s.Logger != nil && !isClosedConnError(err) {
				s.Logger.Println("emux: Clear: send command", "err", err)
			}
		}

		for interval := time.Millisecond; interval < time.Second &&
			atomic.LoadInt64(&s.writerCount) != 0; interval <<= 1 {
			time.Sleep(interval)
		}

		s.writerMut.Lock()
		defer s.writerMut.Unlock()
		s.flush(true)
		close(done)
	}()

	timer := time.NewTimer(s.Timeout)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
		if s.Logger != nil {
			s.Logger.Println("emux: Clear: send close command timeout")
		}
		return
	}

	select {
	case <-s.peerClose:
	case <-timer.C:
		if s.Logger != nil {
			s.Logger.Println("emux: Clear: wait peer close command timeout")
		}
		return
	}
}

func (s *session) IsClosed() bool {
	return atomic.LoadUint32(&s.isClose) == 1
}

func (s *session) Close() error {
	if !atomic.CompareAndSwapUint32(&s.isClose, 0, 1) {
		return nil
	}
	s.Clear()

	s.freeAllStream()
	return s.stm.Close()
}

func (s *session) newStream(sid uint64, idPool *idPool, cli bool) *stream {
	s.mut.Lock()
	defer s.mut.Unlock()
	stm := newStream(s, sid, idPool, cli)
	s.sess[sid] = stm
	return stm
}

func (s *session) checkStream(sid uint64) error {
	if sid == 0 {
		return errUnknownStreamID
	}
	s.mut.RLock()
	defer s.mut.RUnlock()
	stm := s.sess[sid]
	if stm != nil {
		return errStreamAlreadyExists
	}
	return nil
}

func (s *session) freeAllStream() {
	s.mut.Lock()
	defer s.mut.Unlock()
	if len(s.sess) > 0 {
		for _, stm := range s.sess {
			stm.shutdown()
		}
		s.sess = map[uint64]*stream{}
	}
}

func (s *session) freeStream(sid uint64) {
	s.mut.Lock()
	defer s.mut.Unlock()
	stm := s.sess[sid]
	if stm != nil {
		stm.shutdown()
		delete(s.sess, sid)
	}
}

func (s *session) getStream(sid uint64) *stream {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.sess[sid]
}

func (s *session) handleDisconnect(cmd uint8, sid uint64) error {
	if s.IsClosed() {
		return ErrClosed
	}
	stm := s.getStream(sid)
	if stm == nil {
		if s.Logger != nil {
			s.Logger.Println("emux: disconnected stream: get stream", "cmd", s.instruction.Info(cmd), "sid", sid, "err", errUnknownStreamID)
		}
		return s.execDisconnected(sid)
	}

	err := stm.disconnected()
	if err != nil {
		if s.Logger != nil && !isClosedConnError(err) {
			s.Logger.Println("emux: disconnected stream", "cmd", s.instruction.Info(cmd), "sid", sid, "err", err)
		}
		return err
	}
	s.freeStream(sid)
	return nil
}

func (s *session) handleData(cmd uint8, sid uint64, buf []byte) error {
	if s.IsClosed() {
		return ErrClosed
	}
	i, err := s.decode.ReadUvarint()
	if err == nil && i == 0 {
		err = errNoData
	}
	if err != nil {
		if s.Logger != nil && !isClosedConnError(err) {
			s.Logger.Println("emux: handle data: read body length", "cmd", s.instruction.Info(cmd), "sid", sid, "err", err)
		}
		return err
	}

	r := limitReaders.Get(s.decode, int64(i))
	defer limitReaders.Put(r)
	stm := s.getStream(sid)
	if stm == nil {
		n, err := io.CopyBuffer(io.Discard, r, buf)
		if err != nil {
			if s.Logger != nil && !isClosedConnError(err) {
				s.Logger.Println("emux: handle data: write to discard", "cmd", s.instruction.Info(cmd), "sid", sid, "err", err)
			}
			return err
		}
		if n != int64(i) {
			err = errShortRead
			if s.Logger != nil {
				s.Logger.Println("emux: handle data: read body length", "cmd", s.instruction.Info(cmd), "sid", sid, "err", err)
			}
			return err
		}
		return nil
	}

	n, err := io.CopyBuffer(stm.writer, r, buf)
	if err != nil {
		if s.Logger != nil && !isClosedConnError(err) {
			s.Logger.Println("emux: handle data: write to stream", "cmd", s.instruction.Info(cmd), "sid", sid, "err", err)
		}
		return err
	}
	if n != int64(i) {
		err = errShortRead
		if s.Logger != nil {
			s.Logger.Println("emux: handle data: read body length", "cmd", s.instruction.Info(cmd), "sid", sid, "err", err)
		}
		return err
	}
	return nil
}

func (s *session) init() {
	s.decode = NewDecode(readers.Get(s.stm))
	s.encode = NewEncode(writers.Get(s.stm))
}

func (s *session) handleLoop(connectFunc, connectedFunc func(cmd uint8, sid uint64) error) {
	var oncePeerClose sync.Once
	peerCloseFunc := func() {
		oncePeerClose.Do(func() {
			close(s.peerClose)
			s.Clear()
		})
	}
	go s.flushLoop()
	defer func() {
		peerCloseFunc()

		s.writerMut.Lock()
		defer s.writerMut.Unlock()
		writers.Put(s.encode.Writer)
		readers.Put(s.decode.Reader)
		s.decode = nil
		s.encode = nil
	}()
	var buf []byte
	if s.BytesPool != nil {
		buf = s.BytesPool.Get()
		defer s.BytesPool.Put(buf)
	} else {
		buf = make([]byte, bufSize)
	}

	for !s.IsClosed() {
		s.updateTimeout()
		cmd, err := s.decode.ReadByte()
		if err != nil {
			if s.Logger != nil && !isClosedConnError(err) {
				s.Logger.Println("emux: handle loop: get command", "err", err)
			}
			return
		}
		switch cmd {
		case s.instruction.Close:
			go peerCloseFunc()
			continue
		case s.instruction.Connect, s.instruction.Connected, s.instruction.Disconnect, s.instruction.Disconnected, s.instruction.Data:
		default:
			if s.Logger != nil {
				s.Logger.Println("emux: handle loop: unknown cmd", "cmd", s.instruction.Info(cmd))
			}
			return
		}
		sid, err := s.decode.ReadUvarint()
		if err != nil {
			if s.Logger != nil && !isClosedConnError(err) {
				s.Logger.Println("emux: handle loop: get stream id", "cmd", s.instruction.Info(cmd), "err", err)
			}
			return
		}

		switch cmd {
		case s.instruction.Connect:
			if connectFunc == nil {
				if s.Logger != nil {
					s.Logger.Println("emux: handle loop: server can't handle", "cmd", s.instruction.Info(cmd), "sid", sid)
				}
				return
			} else {
				err := connectFunc(cmd, sid)
				if err != nil {
					return
				}
			}
		case s.instruction.Connected:
			if connectedFunc == nil {
				if s.Logger != nil {
					s.Logger.Println("emux: handle loop: server can't handle", "cmd", s.instruction.Info(cmd), "sid", sid)
				}
				return
			} else {
				err := connectedFunc(cmd, sid)
				if err != nil {
					return
				}
			}
		case s.instruction.Disconnect,
			s.instruction.Disconnected: // when both ends are closed at the same time, CmdDisconnect needs to be treated as CmdDisconnected
			err := s.handleDisconnect(cmd, sid)
			if err != nil {
				return
			}
		case s.instruction.Data:
			err := s.handleData(cmd, sid, buf)
			if err != nil {
				return
			}
		}
	}
}

func (s *session) execCmd(cmd uint8) error {
	s.writerMut.Lock()
	defer s.writerMut.Unlock()
	if s.encode == nil {
		return ErrClosed
	}
	err := s.encode.WriteByte(cmd)
	if err != nil {
		if err == io.ErrClosedPipe {
			err = ErrClosed
		}
		return err
	}

	s.updateTimeout()
	return s.flush(false)
}

func (s *session) exec(cmd uint8, sid uint64) error {
	if s.IsClosed() {
		return ErrClosed
	}
	s.writerMut.Lock()
	defer s.writerMut.Unlock()
	if s.encode == nil {
		return ErrClosed
	}
	err := s.encode.WriteCmd(cmd, sid)
	if err != nil {
		if err == io.ErrClosedPipe {
			err = ErrClosed
		}
		return err
	}

	s.updateTimeout()
	return s.flush(false)
}

func (s *session) execConnect(sid uint64) error {
	return s.exec(s.instruction.Connect, sid)
}

func (s *session) execConnected(sid uint64) error {
	return s.exec(s.instruction.Connected, sid)
}

func (s *session) execDisconnect(sid uint64) error {
	return s.exec(s.instruction.Disconnect, sid)
}

func (s *session) execDisconnected(sid uint64) error {
	return s.exec(s.instruction.Disconnected, sid)
}

func (s *session) writeData(sid uint64, b []byte) error {
	if s.IsClosed() {
		return ErrClosed
	}

	atomic.AddInt64(&s.writerCount, 1)
	defer atomic.AddInt64(&s.writerCount, -1)
	s.writerMut.Lock()
	defer s.writerMut.Unlock()
	if s.encode == nil {
		return ErrClosed
	}
	err := s.encode.WriteCmd(s.instruction.Data, sid)
	if err != nil {
		if err == io.ErrClosedPipe {
			err = ErrClosed
		}
		return err
	}
	err = s.encode.WriteBytes(b)
	if err != nil {
		if err == io.ErrClosedPipe {
			err = ErrClosed
		}
		return err
	}

	s.updateTimeout()
	return s.flush(uint64(len(b)) == s.instruction.MaxDataPacketSize)
}

func (s *session) updateTimeout() {
	if s.Timeout > 0 {
		if !atomic.CompareAndSwapUint32(&s.skipUpdateLastTime, 0, 1) {
			return
		}
		defer atomic.StoreUint32(&s.skipUpdateLastTime, 0)
		if t, ok := s.stm.(deadline); ok {
			if time.Since(s.lastTime) < s.Timeout/2 {
				return
			}
			now := time.Now()
			s.lastTime = now
			t.SetDeadline(now.Add(s.IdleTimeout))
		}
	}
}

func (s *session) flushLoop() {
	for !s.IsClosed() {
		select {
		case <-s.flushCh:
			s.writerMut.Lock()
			s.flush(true)
			s.writerMut.Unlock()
		}
	}
}

func (s *session) flushLater() {
	select {
	case s.flushCh <- struct{}{}:
	default:
	}
}

func (s *session) flush(force bool) error {
	err := s.onceError.Load()
	if err != nil {
		return err
	}
	if s.encode == nil {
		return nil
	}
	flusher, ok := s.encode.Writer.(Flusher)
	if !ok {
		return nil
	}

	if !force {
		buffered := flusher.Buffered()
		writerCount := atomic.LoadInt64(&s.writerCount)
		// There is too little buffered data and more data to follow, so wait for subsequent flush
		if uint64(buffered) < s.instruction.MaxDataPacketSize/2 && writerCount != 0 {
			s.flushLater()
			return nil
		}
	}

	err = flusher.Flush()
	if err != nil {
		s.onceError.Store(err)
	}
	return err
}
