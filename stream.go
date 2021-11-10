package emux

import (
	"context"
	"io"
	"sync"
	"time"
)

const (
	packetSize = 1<<16 - 1 - 1024
)

type stream struct {
	sid    uint64
	w      *Encode
	writer *io.PipeWriter
	*io.PipeReader
	ready   chan struct{}
	close   chan struct{}
	mut     *sync.Mutex
	once    sync.Once
	timeout time.Duration
}

func newStream(writer *Encode, mut *sync.Mutex, sid uint64, timeout time.Duration) *stream {
	r, w := io.Pipe()
	return &stream{
		sid:        sid,
		w:          writer,
		writer:     w,
		PipeReader: r,
		mut:        mut,
		ready:      make(chan struct{}),
		close:      make(chan struct{}),
		timeout:    timeout,
	}
}

func (s *stream) connect(ctx context.Context) error {
	err := s.exec(CmdConnect)
	if err != nil {
		return err
	}
	timer := time.NewTimer(s.timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		s.Close()
		return ctx.Err()
	case <-s.ready:
		return nil
	case <-s.close:
		return ErrClosed
	case <-timer.C:
		return ErrTimeout
	}
}

func (s *stream) connected() error {
	return s.exec(CmdConnected)
}

func (s *stream) Close() error {
	if s.isClose() {
		return nil
	}
	return s.disconnect()
}

func (s *stream) isClose() bool {
	select {
	case <-s.close:
		return true
	default:
		return false
	}
}

func (s *stream) shutdown() {
	s.once.Do(func() {
		close(s.close)
	})
	return
}

func (s *stream) disconnect() error {
	var err error
	s.once.Do(func() {
		err = s.exec(CmdDisconnect)
		close(s.close)
	})
	return err
}

func (s *stream) disconnected() error {
	var err error
	s.once.Do(func() {
		err = s.exec(CmdDisconnected)
		close(s.close)
	})
	return err
}

func (s *stream) Read(b []byte) (int, error) {
	if s.isClose() {
		return 0, ErrClosed
	}
	return s.PipeReader.Read(b)
}

func (s *stream) Write(b []byte) (int, error) {
	if s.isClose() {
		return 0, ErrClosed
	}
	l := len(b)
	for len(b) > packetSize {
		err := s.write(b[:packetSize])
		if err != nil {
			return 0, err
		}
		b = b[packetSize:]
	}
	err := s.write(b)
	if err != nil {
		return 0, err
	}
	return l, nil
}

func (s *stream) write(b []byte) error {
	if s.isClose() {
		return ErrClosed
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	err := s.w.WriteCmd(CmdData, s.sid)
	if err != nil {
		return err
	}
	err = s.w.WriteBytes(b)
	if err != nil {
		return err
	}
	return s.w.Flush()
}

func (s *stream) exec(cmd Cmd) error {
	if s.isClose() {
		return ErrClosed
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	err := s.w.WriteCmd(cmd, s.sid)
	if err != nil {
		return err
	}
	return s.w.Flush()
}
