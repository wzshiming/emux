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
	idPool *idPool
	writer *io.PipeWriter
	reader *io.PipeReader
	ready  chan struct{}
	close  chan struct{}
	once   sync.Once
}

func newStream(sess *session, sid uint64, idPool *idPool, cli bool) *stream {
	r, w := io.Pipe()
	s := &stream{
		sess:   sess,
		sid:    sid,
		idPool: idPool,
		writer: w,
		reader: r,
		close:  make(chan struct{}),
	}
	if cli {
		s.ready = make(chan struct{})
	}
	return s
}

func (s *stream) connect(ctx context.Context) error {
	if s.isClose() {
		return ErrClosed
	}
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
			return ErrClosed
		case <-s.ready:
			return nil
		case <-s.close:
			return ErrClosed
		case <-timer.C:
			s.Close()
			return ErrTimeout
		}
	} else {
		select {
		case <-ctx.Done():
			s.Close()
			return ErrClosed
		case <-s.ready:
			return nil
		case <-s.close:
			return ErrClosed
		}
	}
}

func (s *stream) connected() error {
	if s.isClose() {
		return ErrClosed
	}
	return s.exec(s.sess.instruction.Connected)
}

func (s *stream) OriginStream() io.ReadWriteCloser {
	return s.sess.stm
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

func (s *stream) isReady() bool {
	select {
	case <-s.ready:
		return true
	default:
		return false
	}
}

func (s *stream) shut() {
	close(s.close)
	s.writer.Close()
	if s.idPool != nil {
		s.idPool.Put(s.sid)
	}
}

func (s *stream) shutdown() {
	s.once.Do(func() {
		s.shut()
	})
	return
}

func (s *stream) disconnect() error {
	var err error
	s.once.Do(func() {
		err = s.exec(s.sess.instruction.Disconnect)
		s.shut()
	})
	return err
}

func (s *stream) disconnected() error {
	var err error
	s.once.Do(func() {
		err = s.exec(s.sess.instruction.Disconnected)
		s.shut()
	})
	return err
}

func (s *stream) Read(b []byte) (int, error) {
	if s.isClose() {
		return 0, ErrClosed
	}
	return s.reader.Read(b)
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
	s.sess.writerMut.Lock()
	defer s.sess.writerMut.Unlock()
	err := s.sess.encode.WriteCmd(cmd, s.sid)
	if err != nil {
		return err
	}
	return s.sess.encode.Flush()
}
