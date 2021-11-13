package emux

import (
	"context"
	"net"
	"sync"
)

type ListenerSession struct {
	ctx         context.Context
	cancel      func()
	listener    net.Listener
	conns       chan net.Conn
	startOnce   sync.Once
	BytesPool   BytesPool
	Logger      Logger
	Handshake   Handshake
	Instruction Instruction
}

func NewListener(ctx context.Context, listener net.Listener) *ListenerSession {
	ctx, cancel := context.WithCancel(ctx)
	l := &ListenerSession{
		ctx:         ctx,
		cancel:      cancel,
		listener:    listener,
		conns:       make(chan net.Conn),
		Handshake:   DefaultServerHandshake,
		Instruction: DefaultInstruction,
	}
	return l
}

func (l *ListenerSession) start() {
	go l.run()
}

func (l *ListenerSession) run() {
	defer l.Close()
	for l.ctx.Err() == nil {
		conn, err := l.listener.Accept()
		if err != nil {
			if l.Logger != nil {
				l.Logger.Println("emux: listener: accept:", "err", err)
			}
			return
		}
		go func() {
			if l.Handshake != nil {
				err := l.Handshake.Handshake(l.ctx, conn)
				if err != nil {
					if l.Logger != nil {
						l.Logger.Println("emux: listener: handshake:", "err", err)
					}
					conn.Close()
					return
				}
			}
			err = l.acceptSession(l.ctx, conn)
			if err != nil {
				if l.Logger != nil {
					l.Logger.Println("emux: listener: accept session:", "err", err)
				}
			}
		}()
	}
}

func (l *ListenerSession) acceptSession(ctx context.Context, conn net.Conn) error {
	sess := NewServer(conn, &l.Instruction)
	sess.Logger = l.Logger
	sess.BytesPool = l.BytesPool
	for l.ctx.Err() == nil && !sess.IsClosed() {
		stm, err := sess.Accept(ctx)
		if err != nil {
			return err
		}
		l.conns <- newConn(stm, conn.LocalAddr(), conn.RemoteAddr())
	}
	return nil
}

func (l *ListenerSession) Accept() (net.Conn, error) {
	l.startOnce.Do(l.start)
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.ctx.Done():
		return nil, l.ctx.Err()
	}
}

func (l *ListenerSession) Close() error {
	l.cancel()
	return l.listener.Close()
}

func (l *ListenerSession) Addr() net.Addr {
	return l.listener.Addr()
}
