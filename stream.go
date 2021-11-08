package emux

import (
	"io"
	"sync"
)

const (
	packetSize = 1<<16 - 1 - 1024
)

type stream struct {
	sid    uint64
	w      *Encode
	writer *io.PipeWriter
	*io.PipeReader
	ready chan struct{}
	mut   *sync.Mutex
	once  sync.Once
}

func newStream(writer *Encode, mut *sync.Mutex, sid uint64) *stream {
	r, w := io.Pipe()
	return &stream{
		sid:        sid,
		w:          writer,
		writer:     w,
		PipeReader: r,
		mut:        mut,
		ready:      make(chan struct{}),
	}
}

func (w *stream) connect() error {
	return w.exec(CmdConnect)
}

func (w *stream) connected() error {
	return w.exec(CmdConnected)
}

func (w *stream) Close() error {
	return w.disconnect()
}

func (w *stream) disconnect() error {
	var err error
	w.once.Do(func() {
		err = w.exec(CmdDisconnect)
	})
	return err
}

func (w *stream) disconnected() error {
	var err error
	w.once.Do(func() {
		err = w.exec(CmdDisconnected)
	})
	return err
}

func (w *stream) Write(b []byte) (int, error) {
	l := len(b)
	for len(b) > packetSize {
		err := w.write(b[:packetSize])
		if err != nil {
			return 0, err
		}
		b = b[packetSize:]
	}
	err := w.write(b)
	if err != nil {
		return 0, err
	}
	return l, nil
}

func (w *stream) write(b []byte) error {
	w.mut.Lock()
	defer w.mut.Unlock()
	err := w.w.WriteCmd(CmdData, w.sid)
	if err != nil {
		return err
	}
	err = w.w.WriteBytes(b)
	if err != nil {
		return err
	}
	return w.w.Flush()
}

func (w *stream) exec(cmd Cmd) error {
	w.mut.Lock()
	defer w.mut.Unlock()
	err := w.w.WriteCmd(cmd, w.sid)
	if err != nil {
		return err
	}
	return w.w.Flush()
}
