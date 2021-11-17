package emux

import (
	"context"
	"fmt"
	"io"
	"sync"
)

type Client struct {
	idPool    *idPool
	onceStart sync.Once

	*session
}

func NewClient(ctx context.Context, stm io.ReadWriteCloser, instruction *Instruction) *Client {
	return &Client{
		session: newSession(ctx, stm, instruction),
	}
}

func (c *Client) start() {
	c.idPool = newIDPool()
	go c.handleLoop(nil, c.handleConnected)
}

func (c *Client) Dial() (io.ReadWriteCloser, error) {
	c.onceStart.Do(c.start)
	if c.IsClosed() {
		return nil, ErrClosed
	}
	wc := c.dailStream()
	if wc == nil {
		return nil, fmt.Errorf("emux: no free stream id")
	}
	err := wc.connect(c.ctx)
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
	if c.IsClosed() {
		return nil
	}
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
