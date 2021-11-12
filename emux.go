package emux

import (
	"context"
	"fmt"
	"net"
)

type ListenConfigSession struct {
	listenConfig ListenConfig
	Logger       Logger
	BytesPool    BytesPool
	Handshake    Handshake
	Instruction  Instruction
}

func NewListenerConfig(listener ListenConfig) *ListenConfigSession {
	return &ListenConfigSession{
		listenConfig: listener,
		Handshake:    DefaultServerHandshake,
		Instruction:  DefaultInstruction,
	}
}

type DialerSession struct {
	dialer      Dialer
	localAddr   net.Addr
	remoteAddr  net.Addr
	sess        *Client
	BytesPool   BytesPool
	Logger      Logger
	Handshake   Handshake
	Instruction Instruction
}

func NewDialer(dialer Dialer) *DialerSession {
	return &DialerSession{
		dialer:      dialer,
		Handshake:   DefaultClientHandshake,
		Instruction: DefaultInstruction,
	}
}

func (d *DialerSession) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dialContext(ctx, network, address, 3)
}

func (d *DialerSession) dialContext(ctx context.Context, network, address string, retry int) (net.Conn, error) {
	if d.sess == nil || d.sess.IsClosed() {
		if d.sess != nil {
			d.sess.Close()
			d.sess = nil
		}
		conn, err := d.dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if d.Handshake != nil {
			err := d.Handshake.Handshake(ctx, conn)
			if err != nil {
				conn.Close()
				return nil, err
			}
		}

		sess := NewClient(conn, &d.Instruction)
		sess.Logger = d.Logger
		sess.BytesPool = d.BytesPool
		if err != nil {
			return nil, err
		}
		d.localAddr = conn.LocalAddr()
		d.remoteAddr = conn.RemoteAddr()
		d.sess = sess
	}
	stm, err := d.sess.Dial(ctx)
	if err != nil {
		if retry == 0 {
			return nil, err
		}
		if d.sess != nil {
			d.sess.Close()
			d.sess = nil
		}
		return d.dialContext(ctx, network, address, retry-1)
	}
	return newConn(stm, d.localAddr, d.remoteAddr), nil
}

func (l *ListenConfigSession) Listen(ctx context.Context, network, address string) (net.Listener, error) {
	if l.listenConfig == nil {
		return nil, fmt.Errorf("does not support the listen")
	}
	listener, err := l.listenConfig.Listen(ctx, network, address)
	if err != nil {
		return nil, err
	}
	lt := NewListener(ctx, listener)
	lt.Logger = l.Logger
	lt.BytesPool = l.BytesPool
	lt.Handshake = l.Handshake
	lt.Instruction = l.Instruction
	return lt, nil
}

type ListenerSession struct {
	ctx         context.Context
	cancel      func()
	listener    net.Listener
	conns       chan net.Conn
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
	go l.run()
	return l
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
