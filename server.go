package emux

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
)

type Server struct {
	acceptChan chan *stream
	onceStart  sync.Once
	isClose    uint32

	*session
}

func NewServer(ctx context.Context, stm io.ReadWriteCloser, instruction *Instruction) *Server {
	return &Server{
		session: newSession(ctx, stm, instruction),
	}
}

func (s *Server) IsClosed() bool {
	return s.session.IsClosed() || atomic.LoadUint32(&s.isClose) == 1
}

func (s *Server) Close() error {
	if !atomic.CompareAndSwapUint32(&s.isClose, 0, 1) {
		return nil
	}
	s.session.Close()
	if s.acceptChan != nil {
		close(s.acceptChan)
	}
	return nil
}

func (s *Server) start() {
	s.acceptChan = make(chan *stream, 0)
	go s.handleLoop(s.handleConnect, nil)
}

func (s *Server) Accept() (io.ReadWriteCloser, error) {
	s.onceStart.Do(s.start)
	if s.IsClosed() {
		return nil, ErrClosed
	}
	select {
	case <-s.ctx.Done():
		return nil, ErrClosed
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
	if s.IsClosed() {
		return nil
	}
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
		return err
	}
	select {
	case <-s.ctx.Done():
		return ErrClosed
	case s.acceptChan <- stm:
		return nil
	}
}
