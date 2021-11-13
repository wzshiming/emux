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
	ErrClosed = net.ErrClosed
)

type session struct {
	mut sync.RWMutex

	sess map[uint64]*stream

	decode    *Decode
	encode    *Encode
	writerMut sync.Mutex
	closer    io.Closer

	isClose uint32

	instruction *Instruction

	Timeout   time.Duration
	Logger    Logger
	BytesPool BytesPool
}

func newSession(s io.ReadWriteCloser, instruction *Instruction) session {
	reader := readers.Get(s)
	writer := writers.Get(s)
	return session{
		sess:        map[uint64]*stream{},
		decode:      NewDecode(reader),
		encode:      NewEncode(writer),
		instruction: instruction,
		closer:      s,
		Timeout:     10 * time.Second,
	}
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
	for _, stm := range s.sess {
		stm.writer.Close()
		stm.shutdown()
	}
	s.closer.Close()
	return nil
}

func (s *session) newStream(sid uint64, cli bool) *stream {
	s.mut.Lock()
	defer s.mut.Unlock()
	stm := newStream(s.encode, s.instruction, &s.writerMut, sid, s.Timeout, cli)
	s.sess[sid] = stm
	return stm
}

func (s *session) checkStream(sid uint64) error {
	s.mut.RLock()
	defer s.mut.RUnlock()
	stm := s.sess[sid]
	if stm != nil {
		return fmt.Errorf("stream %d already exists", sid)
	}
	return nil
}

func (s *session) freeStream(sid uint64) {
	s.mut.Lock()
	defer s.mut.Unlock()
	stm := s.sess[sid]
	if stm != nil {
		stm.writer.Close()
		delete(s.sess, sid)
	}
}

func (s *session) getStream(sid uint64) *stream {
	s.mut.RLock()
	defer s.mut.RUnlock()

	return s.sess[sid]
}

func (s *session) handleDisconnect(cmd uint8, sid uint64) {
	stm := s.getStream(sid)
	if stm == nil {
		if s.Logger != nil {
			s.Logger.Println("emux: get stream", "cmd", cmd, "sid", sid, "err", "unknown stream id")
		}
		return
	}

	err := stm.disconnected()
	if err != nil {
		if s.Logger != nil {
			s.Logger.Println("emux: disconnected stream error", "cmd", cmd, "sid", sid, "err", err)
		}
		return
	}
	s.freeStream(sid)
	return
}

func (s *session) handleData(cmd uint8, sid uint64, buf []byte) error {
	stm := s.getStream(sid)
	if stm == nil {
		_, err := s.decode.WriteTo(io.Discard, buf)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Println("emux: write to discard error", "cmd", cmd, "sid", sid, "err", err)
			}
		}
		if errors.Is(err, ErrInvalidStream) {
			return err
		}
		return nil
	}

	_, err := s.decode.WriteTo(stm.writer, buf)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Println("emux: write to stream error", "cmd", cmd, "sid", sid, "err", err)
		}
		if errors.Is(err, ErrInvalidStream) {
			return err
		}
		return nil
	}
	return nil
}

func (s *session) handleLoop(connectFunc, connectedFunc func(cmd uint8, sid uint64) error) {
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
			s.handleDisconnect(cmd, sid)
		case s.instruction.Data:
			err := s.handleData(cmd, sid, buf)
			if err != nil {
				return
			}
		}
	}
}
