package emux

import (
	"io"
	"net"
	"sync/atomic"
	"time"
	"unsafe"
)

func NewConn(stm io.ReadWriteCloser, localAddr net.Addr, remoteAddr net.Addr) net.Conn {
	if c, ok := stm.(net.Conn); ok {
		return c
	}
	return &conn{
		readWriteCloser: stm,
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
	onceError     onceError
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
	return c.close(ErrClosed)
}

func (c *conn) close(e error) error {
	if err := c.onceError.Load(); err != nil {
		return err
	}
	err := c.readWriteCloser.Close()
	if err == nil {
		err = e
	}
	c.onceError.Store(err)
	return err
}

func (c *conn) Read(b []byte) (int, error) {
	if err := c.onceError.Load(); err != nil {
		return 0, err
	}
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
	if err := c.onceError.Load(); err != nil {
		return 0, err
	}
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
