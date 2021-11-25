package emux

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// DefaultHandshake
// 0                                                 5
// +---------+---------+---------+---------+---------+
// |                     "EMUX "                     |
// +---------+---------+---------+---------+---------+
var (
	HandshakeData          = []byte("EMUX ")
	DefaultClientHandshake = NewHandshake(HandshakeData, true)
	DefaultServerHandshake = NewHandshake(HandshakeData, false)
	DefaultTimeout         = 600 * time.Second
)

type Handshake interface {
	Handshake(ctx context.Context, rw io.ReadWriter) error
}

type handshake struct {
	mut     sync.Mutex
	bufPool sync.Pool
	data    []byte
	send    bool
}

func NewHandshake(h []byte, send bool) Handshake {
	return &handshake{
		bufPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, len(h))
			},
		},
		send: send,
		data: h,
	}
}

func (h *handshake) Handshake(ctx context.Context, rw io.ReadWriter) error {
	if ctx == context.Background() {
		return h.handshake(rw)
	}
	done := make(chan struct{})
	var err error
	go func() {
		err = h.handshake(rw)
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ErrClosed
	case <-done:
		return err
	}
}

func (h *handshake) handshake(rw io.ReadWriter) error {
	if h.send {
		_, err := rw.Write(h.data)
		if err != nil {
			return err
		}
	}

	buf := h.bufPool.Get().([]byte)
	defer h.bufPool.Put(buf)
	_, err := io.ReadFull(rw, buf)
	if err != nil {
		return err
	}
	if !bytes.Equal(h.data, buf) {
		return fmt.Errorf("handshake error %q", buf)
	}

	if !h.send {
		_, err = rw.Write(h.data)
		if err != nil {
			return err
		}
	}

	return nil
}
