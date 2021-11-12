package emux

import (
	"context"
	"fmt"
	"io"
)

type Client struct {
	idPool *idPool

	session
}

func NewClient(s io.ReadWriteCloser, instruction *Instruction) *Client {
	sess := &Client{
		idPool:  newIDPool(),
		session: newSession(s, instruction),
	}
	go sess.run()
	return sess
}

func (c *Client) run() {
	c.handleLoop(nil, c.handleConnected)
	c.Close()
}

func (c *Client) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	if c.IsClosed() {
		return nil, fmt.Errorf("Client is closed")
	}
	wc := c.dailStream()
	if wc == nil {
		return nil, fmt.Errorf("emux: no free stream id")
	}
	err := wc.connect(ctx)
	if err != nil {
		return nil, err
	}
	return wc, nil
}

func (c *Client) dailStream() *stream {
	sid := c.idPool.Get()
	if sid == 0 {
		return nil
	}
	return c.newStream(sid, true)
}

func (c *Client) handleConnected(cmd uint8, sid uint64) error {
	stm := c.getStream(sid)
	if stm == nil {
		if c.Logger != nil {
			c.Logger.Println("emux: get stream", "cmd", cmd, "sid", sid, "err", "unknown stream id")
		}
		return nil
	}
	close(stm.ready)
	return nil
}
