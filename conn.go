package emux

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	ErrTimeout = fmt.Errorf("timeout")
)

func newConn(stream io.ReadWriteCloser, localAddr net.Addr, remoteAddr net.Addr) net.Conn {
	return &conn{
		readWriteCloser: stream,
		localAddr:       localAddr,
		remoteAddr:      remoteAddr,
	}
}

type conn struct {
	localAddr       net.Addr
	remoteAddr      net.Addr
	readWriteCloser io.ReadWriteCloser

	readDeadline  *time.Time
	writeDeadline *time.Time
	once          sync.Once
	err           error
}

func (c *conn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *conn) SetDeadline(t time.Time) error {
	c.SetWriteDeadline(t)
	c.SetReadDeadline(t)
	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&c.readDeadline)), nil)
	} else {
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&c.readDeadline)), unsafe.Pointer(&t))
	}
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	if t.IsZero() {
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&c.writeDeadline)), nil)
	} else {
		atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&c.writeDeadline)), unsafe.Pointer(&t))
	}
	return nil
}

func (c *conn) Close() error {
	return c.close(nil)
}

func (c *conn) close(err error) error {
	c.once.Do(func() {
		c.err = c.readWriteCloser.Close()
		if c.err == nil {
			c.err = err
		}
	})
	return c.err
}

func (c *conn) Read(b []byte) (int, error) {
	d := (*time.Time)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&c.writeDeadline))))
	if d == nil {
		return c.readWriteCloser.Read(b)
	}
	timer := time.NewTimer(time.Until(*d))
	defer timer.Stop()

	var n int
	var err error
	done := make(chan struct{})
	go func() {
		n, err = c.readWriteCloser.Read(b)
		close(done)
	}()
	select {
	case <-timer.C:
		return 0, c.close(ErrTimeout)
	case <-done:
		return n, err
	}
}

func (c *conn) Write(b []byte) (int, error) {
	d := (*time.Time)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&c.writeDeadline))))
	if d == nil {
		return c.readWriteCloser.Write(b)
	}
	timer := time.NewTimer(time.Until(*d))
	defer timer.Stop()

	var n int
	var err error
	done := make(chan struct{})
	go func() {
		n, err = c.readWriteCloser.Write(b)
		close(done)
	}()
	select {
	case <-timer.C:
		return 0, c.close(ErrTimeout)
	case <-done:
		return n, err
	}
}
