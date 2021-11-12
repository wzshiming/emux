package emux

import (
	"context"
	"fmt"
	"io"
	"net"
)

type Server struct {
	acceptChan chan *stream

	session
}

func NewServer(s io.ReadWriteCloser, instruction *Instruction) *Server {
	sess := &Server{
		acceptChan: make(chan *stream, 0),
		session:    newSession(s, instruction),
	}
	go sess.run()
	return sess
}

func (s *Server) run() {
	s.handleLoop(s.handleConnect, nil)
	s.Close()
}

func (s *Server) Accept(ctx context.Context) (io.ReadWriteCloser, error) {
	if s.IsClosed() {
		return nil, fmt.Errorf("Server is closed")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case conn, ok := <-s.acceptChan:
		if !ok {
			return nil, net.ErrClosed
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
