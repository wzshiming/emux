package emux

import (
	"bytes"
	"fmt"
	"io"
	"sync"
)

// DefaultHandshake
// 0                                                 5
// +---------+---------+---------+---------+---------+
// |                     "EMUX "                     |
// +---------+---------+---------+---------+---------+
var (
	HandshakeData    = []byte("EMUX ")
	DefaultHandshake = NewHandshake(HandshakeData)
)

type Handshake interface {
	Handshake(rw io.ReadWriter) error
}

type handshake struct {
	mut     sync.Mutex
	bufPool sync.Pool
	data    []byte
}

func NewHandshake(h []byte) Handshake {
	return &handshake{
		bufPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, len(h))
			},
		},
		data: h,
	}
}

func (h *handshake) Handshake(rw io.ReadWriter) error {
	_, err := rw.Write(h.data)
	if err != nil {
		return err
	}

	buf := h.bufPool.Get().([]byte)
	defer h.bufPool.Put(buf)
	_, err = io.ReadFull(rw, buf)
	if err != nil {
		return err
	}
	if !bytes.Equal(h.data, buf) {
		return fmt.Errorf("handshake error %q", buf)
	}
	return nil
}
