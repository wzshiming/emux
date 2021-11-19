package emux

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type ListenerSession struct {
	ctx         context.Context
	cancel      func()
	listener    net.Listener
	conns       chan net.Conn
	startOnce   sync.Once
	isClose     uint32
	BytesPool   BytesPool
	Logger      Logger
	Handshake   Handshake
	Instruction Instruction
	Timeout     time.Duration
}

func NewListener(ctx context.Context, listener net.Listener) *ListenerSession {
	ctx, cancel := context.WithCancel(ctx)
	l := &ListenerSession{
		ctx:         ctx,
		cancel:      cancel,
		listener:    listener,
		Handshake:   DefaultServerHandshake,
		Instruction: DefaultInstruction,
		Timeout:     DefaultTimeout,
	}
	return l
}

func (l *ListenerSession) start() {
	l.conns = make(chan net.Conn)
	go l.run()
}

func (l *ListenerSession) run() {
	defer l.Close()
	for l.ctx.Err() == nil && !l.IsClosed() {
		conn, err := l.listener.Accept()
		if err != nil {
			if l.Logger != nil {
				l.Logger.Println("emux: listener: accept:", "err", err)
			}
			return
		}
		go l.acceptSession(conn)
	}
}

func (l *ListenerSession) acceptSession(conn net.Conn) {
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
	sess := NewServer(l.ctx, conn, &l.Instruction)
	sess.Logger = l.Logger
	sess.BytesPool = l.BytesPool
	sess.Timeout = l.Timeout
	defer sess.Close()
	for l.ctx.Err() == nil && !sess.IsClosed() {
		stm, err := sess.Accept()
		if err != nil {
			if l.Logger != nil {
				l.Logger.Println("emux: listener: accept session:", "err", err)
			}
			return
		}
		conn := newConn(stm, conn.LocalAddr(), conn.RemoteAddr())
		select {
		case <-l.ctx.Done():
			conn.Close()
			return
		case l.conns <- conn:
		}
	}
}

func (l *ListenerSession) Accept() (net.Conn, error) {
	l.startOnce.Do(l.start)
	if l.IsClosed() {
		return nil, ErrClosed
	}
	select {
	case <-l.ctx.Done():
		close(l.conns)
		return nil, ErrClosed
	case conn, ok := <-l.conns:
		if !ok {
			return nil, ErrClosed
		}
		return conn, nil
	}
}

func (l *ListenerSession) IsClosed() bool {
	return atomic.LoadUint32(&l.isClose) == 1
}

func (l *ListenerSession) Close() error {
	if !atomic.CompareAndSwapUint32(&l.isClose, 0, 1) {
		return nil
	}
	l.cancel()
	return l.listener.Close()
}

func (l *ListenerSession) Addr() net.Addr {
	return l.listener.Addr()
}
