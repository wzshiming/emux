package emux

import (
	"context"
	"io"
	"sync"
)

type Server struct {
	acceptChan chan io.ReadWriteCloser
	onceStart  sync.Once

	*session
}

func NewServer(ctx context.Context, stm io.ReadWriteCloser, instruction *Instruction) *Server {
	return &Server{
		session: newSession(ctx, stm, instruction),
	}
}

func (s *Server) start() {
	if s.acceptChan == nil {
		s.acceptChan = make(chan io.ReadWriteCloser, 0)
	}
	s.init()
	go s.handleLoop(s.handleConnect, nil)
}

func (s *Server) Accept() (io.ReadWriteCloser, error) {
	s.onceStart.Do(s.start)
	if s.IsClosed() {
		return nil, ErrClosed
	}
	select {
	case <-s.ctx.Done():
		close(s.acceptChan)
		return nil, ErrClosed
	case conn, ok := <-s.acceptChan:
		if !ok {
			return nil, ErrClosed
		}
		return conn, nil
	}
}

func (s *Server) AcceptTo(acceptChan chan io.ReadWriteCloser) error {
	if s.acceptChan != nil {
		return ErrAlreadyStarted
	}
	s.acceptChan = acceptChan
	s.onceStart.Do(s.start)
	return nil
}

func (s *Server) acceptStream(sid uint64) *stream {
	return s.newStream(sid, nil, false)
}

func (s *Server) handleConnect(cmd uint8, sid uint64) error {
	if s.IsClosed() {
		return ErrClosed
	}
	err := s.checkStream(sid)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Println("emux: check stream", "cmd", cmd, "sid", sid, "err", err)
		}
		return err
	}
	stm := s.acceptStream(sid)
	err = stm.connected()
	if err != nil {
		if s.Logger != nil {
			s.Logger.Println("emux: connected", "cmd", cmd, "sid", sid, "err", err)
		}
		return err
	}
	select {
	case <-s.ctx.Done():
		stm.Close()
		return ErrClosed
	case s.acceptChan <- stm:
		return nil
	}
}
