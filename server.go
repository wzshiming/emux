package emux

import (
	"context"
	"io"
	"sync"
)

type Server struct {
	acceptChan chan *stream
	onceStart  sync.Once

	*session
}

func NewServer(s io.ReadWriteCloser, instruction *Instruction) *Server {
	sess := &Server{
		acceptChan: make(chan *stream, 0),
		session:    newSession(s, instruction),
	}
	return sess
}

func (s *Server) start() {
	go s.handleLoop(s.handleConnect, nil)
}

func (s *Server) Accept(ctx context.Context) (io.ReadWriteCloser, error) {
	s.onceStart.Do(s.start)
	if s.IsClosed() {
		return nil, ErrClosed
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn, ok := <-s.acceptChan:
		if !ok {
			return nil, ErrClosed
		}
		return conn, nil
	}
}

func (s *Server) acceptStream(sid uint64) *stream {
	return s.newStream(sid, false)
}

func (s *Server) handleConnect(cmd uint8, sid uint64) error {
	err := s.checkStream(sid)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Println("emux: check stream", "cmd", cmd, "sid", sid, "err", err)
		}
		return nil
	}
	stm := s.acceptStream(sid)
	err = stm.connected()
	if err != nil {
		if s.Logger != nil {
			s.Logger.Println("emux: connected", "cmd", cmd, "sid", sid, "err", err)
		}
		return nil
	}
	s.acceptChan <- stm
	return nil
}
