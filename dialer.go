package emux

import (
	"context"
	"net"
	"sync"
	"time"
)

type DialerSession struct {
	ctx         context.Context
	mut         sync.Mutex
	dialer      Dialer
	localAddr   net.Addr
	remoteAddr  net.Addr
	sess        *Client
	BytesPool   BytesPool
	Logger      Logger
	Handshake   Handshake
	Instruction Instruction
	Timeout     time.Duration
	IdleTimeout time.Duration
	Retry       int
}

func NewDialer(ctx context.Context, dialer Dialer) *DialerSession {
	return &DialerSession{
		ctx:         ctx,
		dialer:      dialer,
		Handshake:   DefaultClientHandshake,
		Instruction: DefaultInstruction,
		Timeout:     DefaultTimeout,
		IdleTimeout: DefaultIdleTimeout,
		Retry:       3,
	}
}

func (d *DialerSession) Close() error {
	d.mut.Lock()
	defer d.mut.Unlock()
	if d.sess != nil {
		d.sess.Close()
		d.sess = nil
	}
	return nil
}

func (d *DialerSession) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d.mut.Lock()
	defer d.mut.Unlock()
	return d.dialContext(ctx, network, address, d.Retry)
}

func (d *DialerSession) dialContext(ctx context.Context, network, address string, retry int) (net.Conn, error) {
	if d.sess == nil || d.sess.IsClear() {
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

		sess := NewClient(d.ctx, conn, &d.Instruction)
		sess.Logger = d.Logger
		sess.BytesPool = d.BytesPool
		sess.Timeout = d.Timeout
		sess.IdleTimeout = d.IdleTimeout
		if err != nil {
			return nil, err
		}
		d.localAddr = conn.LocalAddr()
		d.remoteAddr = conn.RemoteAddr()
		d.sess = sess
	}
	stm, err := d.sess.Dial(ctx)
	if err != nil {
		if d.sess != nil {
			d.sess.Close()
			d.sess = nil
		}
		if retry <= 0 || d.ctx.Err() != nil || ctx.Err() != nil {
			return nil, err
		}
		return d.dialContext(ctx, network, address, retry-1)
	}
	return newConn(stm, d.localAddr, d.remoteAddr), nil
}
