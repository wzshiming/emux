package emux

import (
	"container/ring"
	"sync"
)

var (
	ringPools = sync.Pool{
		New: func() interface{} {
			return ring.New(1)
		},
	}
)

type idPool struct {
	ring  *ring.Ring
	count uint64
	index uint64
}

func newIDPool() *idPool {
	i := &idPool{}
	return i
}

func (s *idPool) Get() uint64 {
	// less than 0x80 , that StreamID is takes only one byte
	if s.index >= 0x80 {
		s.index++
		return s.index
	}

	// when the id in the pool is too few, no longer use pool, avoid duplicate
	if s.count < 0x40 {
		s.index++
		return s.index
	}
	s.count--
	r := s.ring.Unlink(1)
	v := r.Value.(uint64)
	ringPools.Put(r)
	return v
}

func (s *idPool) Put(id uint64) {
	r := ringPools.Get().(*ring.Ring)
	r.Value = id
	if s.ring == nil {
		s.ring = r
	} else {
		s.ring.Link(r)
		s.ring = r
	}
	s.count++
}
