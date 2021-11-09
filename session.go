package emux

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrClosed = errors.New("use of closed network connection")
)

type Session struct {
	mut sync.RWMutex

	idPool *idPool
	sess   map[uint64]*stream

	decode    *Decode
	encode    *Encode
	writerMut sync.Mutex
	closer    io.Closer

	acceptChan chan *stream

	isClose uint32

	Timeout time.Duration

	Logger    Logger
	BytesPool BytesPool
}

func NewSession(s io.ReadWriteCloser) *Session {
	reader := readers.Get(s)
	writer := writers.Get(s)
	sess := &Session{
		idPool:     newIDPool(),
		sess:       map[uint64]*stream{},
		decode:     NewDecode(reader),
		encode:     NewEncode(writer),
		closer:     s,
		Timeout:    10 * time.Second,
		acceptChan: make(chan *stream, 0),
	}
	go sess.run()
	return sess
}

func (s *Session) run() {
	s.handleLoop()
	s.Close()
}

func (s *Session) IsClosed() bool {
	return atomic.LoadUint32(&s.isClose) == 1
}

func (s *Session) Close() error {
	if !atomic.CompareAndSwapUint32(&s.isClose, 0, 1) {
		return nil
	}

	s.mut.Lock()
	defer s.mut.Unlock()
	s.writerMut.Lock()
	defer s.writerMut.Unlock()
	close(s.acceptChan)
	s.encode.WriteByte(byte(CmdClose))
	for _, stm := range s.sess {
		stm.writer.Close()
		stm.shutdown()
	}
	s.closer.Close()
	return nil
}

func (s *Session) Accept() (io.ReadWriteCloser, error) {
	if s.IsClosed() {
		return nil, fmt.Errorf("session is closed")
	}
	conn, ok := <-s.acceptChan
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (s *Session) Open() (io.ReadWriteCloser, error) {
	if s.IsClosed() {
		return nil, fmt.Errorf("session is closed")
	}
	wc := s.openStream()
	if wc == nil {
		return nil, fmt.Errorf("emux: no free stream id")
	}
	err := wc.connect()
	if err != nil {
		return nil, err
	}
	return wc, nil
}

func (s *Session) openStream() *stream {
	s.mut.Lock()
	defer s.mut.Unlock()

	sid := s.idPool.Get()
	if sid == 0 {
		return nil
	}

	stm := newStream(s.encode, &s.writerMut, sid, s.Timeout)
	s.sess[sid] = stm
	return stm
}

func (s *Session) acceptStream(sid uint64) *stream {
	s.mut.Lock()
	defer s.mut.Unlock()

	stm := newStream(s.encode, &s.writerMut, sid, s.Timeout)
	s.sess[sid] = stm
	return stm
}

func (s *Session) freeStream(sid uint64) {
	s.mut.Lock()
	defer s.mut.Unlock()
	stm := s.sess[sid]
	if stm != nil {
		stm.writer.Close()
		delete(s.sess, sid)
		s.idPool.Put(sid)
	}
}

func (s *Session) getStream(sid uint64) *stream {
	s.mut.RLock()
	defer s.mut.RUnlock()

	return s.sess[sid]
}

func (s *Session) handleLoop() {
	defer func() {
		s.Close()
		readers.Put(s.decode.r.(*bufio.Reader))
		writers.Put(s.encode.w.(*bufio.Writer))
	}()
	var buf []byte
	if s.BytesPool != nil {
		buf = s.BytesPool.Get()
		defer s.BytesPool.Put(buf)
	} else {
		buf = make([]byte, 32*1024)
	}
	for !s.IsClosed() {
		c, err := s.decode.ReadByte()
		if err != nil {
			if s.Logger != nil {
				s.Logger.Println("emux: handleLoop:", "err", err)
			}
			return
		}
		cmd := Cmd(c)
		switch cmd {
		case CmdClose:
			return
		case CmdConnect, CmdConnected, CmdDisconnect, CmdDisconnected, CmdData:
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
		case CmdConnect:
			stm := s.getStream(sid)
			if stm != nil {
				if s.Logger != nil {
					s.Logger.Println("emux: get stream", "cmd", cmd, "sid", sid, "err", "duplicate sid")
				}
				continue
			}
			stm = s.acceptStream(sid)
			err := stm.connected()
			if err != nil {
				if s.Logger != nil {
					s.Logger.Println("emux: connected", "cmd", cmd, "sid", sid, "err", err)
				}
				continue
			}
			s.acceptChan <- stm
		case CmdConnected:
			stm := s.getStream(sid)
			if stm == nil {
				if s.Logger != nil {
					s.Logger.Println("emux: get stream", "cmd", cmd, "sid", sid, "err", "unknown stream id")
				}
				continue
			}
			close(stm.ready)
		case CmdDisconnect,
			CmdDisconnected: // when both ends are closed at the same time, CmdDisconnect needs to be treated as CmdDisconnected
			stm := s.getStream(sid)
			if stm == nil {
				if s.Logger != nil {
					s.Logger.Println("emux: get stream", "cmd", cmd, "sid", sid, "err", "unknown stream id")
				}
				continue
			}

			err = stm.disconnected()
			if err != nil {
				if s.Logger != nil {
					s.Logger.Println("emux: disconnected stream error", "cmd", cmd, "sid", sid, "err", err)
				}
				continue
			}
			s.freeStream(sid)
		case CmdData:
			stm := s.getStream(sid)
			if stm == nil {
				_, err := s.decode.WriteTo(io.Discard, buf)
				if err != nil {
					if s.Logger != nil {
						s.Logger.Println("emux: write to discard error", "cmd", cmd, "sid", sid, "err", err)
					}
				}
				if errors.Is(err, ErrInvalidStream) {
					return
				}
				continue
			}

			_, err := s.decode.WriteTo(stm.writer, buf)
			if err != nil {
				if s.Logger != nil {
					s.Logger.Println("emux: write to stream error", "cmd", cmd, "sid", sid, "err", err)
				}
				if errors.Is(err, ErrInvalidStream) {
					return
				}
				continue
			}
		}
	}
}
