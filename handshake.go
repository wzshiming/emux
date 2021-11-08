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
	mut  sync.Mutex
	buf  []byte
	data []byte
}

func NewHandshake(h []byte) Handshake {
	return &handshake{
		buf:  make([]byte, len(h)),
		data: h,
	}
}

func (h *handshake) Handshake(rw io.ReadWriter) error {
	_, err := rw.Write(h.data)
	if err != nil {
		return err
	}

	h.mut.Lock()
	defer h.mut.Unlock()
	_, err = io.ReadFull(rw, h.buf)
	if err != nil {
		return err
	}
	if !bytes.Equal(h.data, h.buf) {
		return fmt.Errorf("handshake error %q", h.buf)
	}
	return nil
}
