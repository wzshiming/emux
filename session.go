package emux

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrClosed               = net.ErrClosed
	ErrAlreadyStarted       = fmt.Errorf("session already started")
	errUnknownStreamID      = fmt.Errorf("unknown stream id")
	errStreamIsAlreadyReady = fmt.Errorf("stream is already ready")
	errNoFreeStreamID       = fmt.Errorf("emux: no free stream id")
	errShortRead            = fmt.Errorf("read length not equal to body length")
	errStreamAlreadyExists  = fmt.Errorf("stream id already exists")
	errNoData               = fmt.Errorf("data length cannot be zero")
)

type session struct {
	ctx    context.Context
	cancel func()

	mut sync.RWMutex

	sess map[uint64]*stream

	decode      *Decode
	encode      *Encode
	writerMut   sync.Mutex
	stm         io.ReadWriteCloser
	isClose     uint32
	instruction *Instruction

	Timeout   time.Duration
	Logger    Logger
	BytesPool BytesPool
}

func newSession(ctx context.Context, stm io.ReadWriteCloser, instruction *Instruction) *session {
	ctx, cancel := context.WithCancel(ctx)
	reader := readers.Get(stm)
	writer := writers.Get(stm)
	s := &session{
		ctx:         ctx,
		cancel:      cancel,
		sess:        map[uint64]*stream{},
		decode:      NewDecode(reader),
		encode:      NewEncode(writer),
		instruction: instruction,
		stm:         stm,
		Timeout:     DefaultTimeout,
	}
	return s
}

func (s *session) IsClosed() bool {
	return atomic.LoadUint32(&s.isClose) == 1
}

func (s *session) Close() error {
	if !atomic.CompareAndSwapUint32(&s.isClose, 0, 1) {
		return nil
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	s.writerMut.Lock()
	defer s.writerMut.Unlock()

	s.encode.WriteByte(s.instruction.Close)
	s.stm.Close()
	s.cancel()
	for _, stm := range s.sess {
		stm.shutdown()
	}

	readers.Put(s.decode.DecodeReader)
	writers.Put(s.encode.EncodeWriter)
	return nil
}

func (s *session) newStream(sid uint64, cli bool) *stream {
	s.mut.Lock()
	defer s.mut.Unlock()
	stm := newStream(s, sid, cli)
	s.sess[sid] = stm
	return stm
}

func (s *session) checkStream(sid uint64) error {
	s.mut.RLock()
	defer s.mut.RUnlock()
	stm := s.sess[sid]
	if stm != nil {
		return errStreamAlreadyExists
	}
	return nil
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
	stm := s.getStream(sid)
	if stm == nil {
		if s.Logger != nil {
			s.Logger.Println("emux: get stream", "cmd", cmd, "sid", sid, "err", errUnknownStreamID)
		}
		s.writerMut.Lock()
		defer s.writerMut.Unlock()
		return s.encode.WriteCmd(s.instruction.Disconnected, sid)
	}

	err := stm.disconnected()
	if err != nil {
		if s.Logger != nil {
			s.Logger.Println("emux: disconnected stream error", "cmd", cmd, "sid", sid, "err", err)
		}
		return err
	}
	s.freeStream(sid)
	return nil
}

func (s *session) handleData(cmd uint8, sid uint64, buf []byte) error {
	i, err := s.decode.ReadUvarint()
	if err == nil && i == 0 {
		err = errNoData
	}
	if err != nil {
		if s.Logger != nil {
			s.Logger.Println("emux: read body length error", "cmd", cmd, "sid", sid, "err", err)
		}
		return err
	}

	r := limitReaders.Get(s.decode, int64(i))
	defer limitReaders.Put(r)
	stm := s.getStream(sid)
	if stm == nil {
		n, err := io.CopyBuffer(io.Discard, r, buf)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Println("emux: write to discard error", "cmd", cmd, "sid", sid, "err", err)
			}
			return err
		}
		if n != int64(i) {
			err = errShortRead
			if s.Logger != nil {
				s.Logger.Println("emux: read body length error", "cmd", cmd, "sid", sid, "err", err)
			}
			return err
		}
		return nil
	}

	n, err := io.CopyBuffer(stm.writer, r, buf)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Println("emux: write to stream error", "cmd", cmd, "sid", sid, "err", err)
		}
		return err
	}
	if n != int64(i) {
		err = errShortRead
		if s.Logger != nil {
			s.Logger.Println("emux: read body length error", "cmd", cmd, "sid", sid, "err", err)
		}
		return err
	}
	return nil
}

func (s *session) handleLoop(connectFunc, connectedFunc func(cmd uint8, sid uint64) error) {
	defer s.Close()
	var buf []byte
	if s.BytesPool != nil {
		buf = s.BytesPool.Get()
		defer s.BytesPool.Put(buf)
	} else {
		buf = make([]byte, bufSize)
	}
	for !s.IsClosed() {
		cmd, err := s.decode.ReadByte()
		if err != nil {
			if s.Logger != nil {
				s.Logger.Println("emux: handleLoop:", "err", err)
			}
			return
		}
		switch cmd {
		case s.instruction.Close:
			return
		case s.instruction.Connect, s.instruction.Connected, s.instruction.Disconnect, s.instruction.Disconnected, s.instruction.Data:
		default:
			if s.Logger != nil {
				s.Logger.Println("emux: handleLoop: unknown cmd", "cmd", cmd)
			}
			return
		}
		sid, err := s.decode.ReadUvarint()
		if err != nil {
			if s.Logger != nil {
				s.Logger.Println("emux: handleLoop:", "cmd", cmd, "err", err)
			}
			return
		}

		switch cmd {
		case s.instruction.Connect:
			if connectFunc == nil {
				if s.Logger != nil {
					s.Logger.Println("emux: server can't handle", "cmd", cmd, "sid", sid)
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
					s.Logger.Println("emux: server can't handle", "cmd", cmd, "sid", sid)
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
