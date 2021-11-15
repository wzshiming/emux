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

func NewClient(s io.ReadWriteCloser, instruction *Instruction) *Client {
	sess := &Client{
		idPool:  newIDPool(),
		session: newSession(s, instruction),
	}
	return sess
}

func (c *Client) start() {
	go c.handleLoop(nil, c.handleConnected)
}

func (c *Client) Dial(ctx context.Context) (io.ReadWriteCloser, error) {
	c.onceStart.Do(c.start)
	if c.IsClosed() {
		return nil, ErrClosed
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
