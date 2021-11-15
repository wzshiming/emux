package emux

import (
	"context"
	"io"
	"sync"
	"time"
)

type stream struct {
	sess   *session
	sid    uint64
	writer *io.PipeWriter
	*io.PipeReader
	ready chan struct{}
	close chan struct{}
	once  sync.Once
}

func newStream(sess *session, sid uint64, cli bool) *stream {
	r, w := io.Pipe()
	s := &stream{
		sess:       sess,
		sid:        sid,
		writer:     w,
		PipeReader: r,
		close:      make(chan struct{}),
	}
	if cli {
		s.ready = make(chan struct{})
	}
	return s
}

func (s *stream) connect(ctx context.Context) error {
	err := s.exec(s.sess.instruction.Connect)
	if err != nil {
		return err
	}
	if s.sess.Timeout > 0 {
		timer := time.NewTimer(s.sess.Timeout)
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
	} else {
		select {
		case <-ctx.Done():
			s.Close()
			return ctx.Err()
		case <-s.ready:
			return nil
		case <-s.close:
			return ErrClosed
		}
	}
}

func (s *stream) connected() error {
	return s.exec(s.sess.instruction.Connected)
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
		s.writer.Close()
	})
	return
}

func (s *stream) disconnect() error {
	var err error
	s.once.Do(func() {
		err = s.exec(s.sess.instruction.Disconnect)
		close(s.close)
	})
	return err
}

func (s *stream) disconnected() error {
	var err error
	s.once.Do(func() {
		err = s.exec(s.sess.instruction.Disconnected)
		close(s.close)
		s.writer.Close()
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
	maxDataPacketSize := s.sess.instruction.MaxDataPacketSize
	for uint64(len(b)) > maxDataPacketSize {
		err := s.write(b[:maxDataPacketSize])
		if err != nil {
			return 0, err
		}
		b = b[maxDataPacketSize:]
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
	s.sess.writerMut.Lock()
	defer s.sess.writerMut.Unlock()
	err := s.sess.encode.WriteCmd(s.sess.instruction.Data, s.sid)
	if err != nil {
		return err
	}
	err = s.sess.encode.WriteBytes(b)
	if err != nil {
		return err
	}
	return s.sess.encode.Flush()
}

func (s *stream) exec(cmd uint8) error {
	if s.isClose() {
		return ErrClosed
	}
	s.sess.writerMut.Lock()
	defer s.sess.writerMut.Unlock()
	err := s.sess.encode.WriteCmd(cmd, s.sid)
	if err != nil {
		return err
	}
	return s.sess.encode.Flush()
}
