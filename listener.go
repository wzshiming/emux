package emux

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type ListenerSession struct {
	ctx         context.Context
	cancel      func()
	listener    net.Listener
	acceptChan  chan io.ReadWriteCloser
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
	l.acceptChan = make(chan io.ReadWriteCloser, 0)
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
	err := sess.AcceptTo(l.acceptChan)
	if err != nil {
		if l.Logger != nil {
			l.Logger.Println("emux: listener: accept:", "err", err)
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
		close(l.acceptChan)
		return nil, ErrClosed
	case stm, ok := <-l.acceptChan:
		if !ok {
			return nil, ErrClosed
		}
		istm, ok := stm.(interface{ OriginStream() io.ReadWriteCloser })
		if !ok {
			stm.Close()
			return l.Accept()
		}
		ostm := istm.OriginStream()
		conn, ok := ostm.(net.Conn)
		if !ok {
			stm.Close()
			return l.Accept()
		}
		return newConn(stm, conn.LocalAddr(), conn.RemoteAddr()), nil
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
