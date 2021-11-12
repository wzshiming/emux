package emux

import (
	"context"
	"io"
	"sync"
	"time"
)

type stream struct {
	sid    uint64
	w      *Encode
	writer *io.PipeWriter
	*io.PipeReader
	ready       chan struct{}
	close       chan struct{}
	mut         *sync.Mutex
	once        sync.Once
	timeout     time.Duration
	instruction *Instruction
}

func newStream(writer *Encode, instruction *Instruction, mut *sync.Mutex, sid uint64, timeout time.Duration, cli bool) *stream {
	r, w := io.Pipe()
	s := &stream{
		sid:         sid,
		w:           writer,
		writer:      w,
		PipeReader:  r,
		mut:         mut,
		close:       make(chan struct{}),
		timeout:     timeout,
		instruction: instruction,
	}
	if cli {
		s.ready = make(chan struct{})
	}
	return s
}

func (s *stream) connect(ctx context.Context) error {
	err := s.exec(s.instruction.Connect)
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
	return s.exec(s.instruction.Connected)
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
		err = s.exec(s.instruction.Disconnect)
		close(s.close)
	})
	return err
}

func (s *stream) disconnected() error {
	var err error
	s.once.Do(func() {
		err = s.exec(s.instruction.Disconnected)
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
	for uint64(len(b)) > s.instruction.MaxDataPacketSize {
		err := s.write(b[:s.instruction.MaxDataPacketSize])
		if err != nil {
			return 0, err
		}
		b = b[s.instruction.MaxDataPacketSize:]
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
	err := s.w.WriteCmd(s.instruction.Data, s.sid)
	if err != nil {
		return err
	}
	err = s.w.WriteBytes(b)
	if err != nil {
		return err
	}
	return s.w.Flush()
}

func (s *stream) exec(cmd uint8) error {
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
