package emux

import (
	"context"
	"net"
)

type DialerSession struct {
	dialer      Dialer
	localAddr   net.Addr
	remoteAddr  net.Addr
	sess        *Client
	BytesPool   BytesPool
	Logger      Logger
	Handshake   Handshake
	Instruction Instruction
	Retry       int
}

func NewDialer(dialer Dialer) *DialerSession {
	return &DialerSession{
		dialer:      dialer,
		Handshake:   DefaultClientHandshake,
		Instruction: DefaultInstruction,
		Retry:       3,
	}
}

func (d *DialerSession) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dialContext(ctx, network, address, d.Retry)
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
		if d.sess != nil {
			d.sess.Close()
			d.sess = nil
		}
		if retry <= 0 {
			return nil, err
		}
		return d.dialContext(ctx, network, address, retry-1)
	}
	return newConn(stm, d.localAddr, d.remoteAddr), nil
}
