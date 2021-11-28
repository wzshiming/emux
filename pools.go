package emux

import (
	"io"
	"sync"
)

var (
	limitReaders = newLimitReaderPool()
)

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
