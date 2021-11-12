package emux

import (
	"bufio"
	"io"
	"sync"
)

var (
	readers = newReaderPool()
	writers = newWriterPool()
)

type readerPool struct {
	pool sync.Pool
}

func newReaderPool() *readerPool {
	return &readerPool{
		pool: sync.Pool{
			New: func() interface{} {
				return bufio.NewReaderSize(nil, bufSize)
			},
		},
	}
}

func (p *readerPool) Get(r io.Reader) *bufio.Reader {
	buf := p.pool.Get().(*bufio.Reader)
	buf.Reset(r)
	return buf
}

func (p *readerPool) Put(buf *bufio.Reader) {
	buf.Reset(nil)
	p.pool.Put(buf)
}

type writerPool struct {
	pool sync.Pool
}

func newWriterPool() *writerPool {
	return &writerPool{
		pool: sync.Pool{
			New: func() interface{} {
				return bufio.NewWriterSize(nil, bufSize)
			},
		},
	}
}

func (p *writerPool) Get(w io.Writer) *bufio.Writer {
	buf := p.pool.Get().(*bufio.Writer)
	buf.Reset(w)
	return buf
}

func (p *writerPool) Put(w *bufio.Writer) {
	w.Reset(nil)
	p.pool.Put(w)
}
