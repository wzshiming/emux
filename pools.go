package emux

import (
	"bufio"
	"io"
	"sync"
)

var (
	readers      = newReaderPool()
	writers      = newWriterPool()
	limitReaders = newLimitReaderPool()
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

func (p *readerPool) Get(r io.Reader) io.Reader {
	if _, ok := r.(ByteReader); ok {
		return r
	}
	buf := p.pool.Get().(*bufio.Reader)
	buf.Reset(r)
	return buf
}

func (p *readerPool) Put(buf io.Reader) {
	reader, ok := buf.(*bufio.Reader)
	if ok {
		reader.Reset(nil)
		p.pool.Put(reader)
	}
}

type limitReaderPool struct {
	pool sync.Pool
}

func newLimitReaderPool() *limitReaderPool {
	return &limitReaderPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &io.LimitedReader{}
			},
		},
	}
}

func (p *limitReaderPool) Get(r io.Reader, n int64) *io.LimitedReader {
	buf := p.pool.Get().(*io.LimitedReader)
	buf.R = r
	buf.N = n
	return buf
}

func (p *limitReaderPool) Put(buf *io.LimitedReader) {
	buf.N = 0
	buf.R = nil
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

func (p *writerPool) Get(w io.Writer) io.Writer {
	if _, ok := w.(Flusher); ok {
		return w
	}
	buf := p.pool.Get().(*bufio.Writer)
	buf.Reset(w)
	return buf
}

func (p *writerPool) Put(w io.Writer) {
	writer, ok := w.(*bufio.Writer)
	if ok {
		writer.Reset(nil)
		p.pool.Put(writer)
	}
}
