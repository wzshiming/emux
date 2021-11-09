package emux

import (
	"testing"
)

func TestIDPool(t *testing.T) {
	pool := newIDPool()
	for i := 0; i <= 0x80; i++ {
		pool.Put(pool.Get())
	}
	p := map[uint64]struct{}{}
	for i := 0; i != 0x40; i++ {
		id := pool.Get()
		_, ok := p[id]
		if ok {
			t.Errorf("id %d already exists", id)
		}
		p[id] = struct{}{}
		pool.Put(id)
	}
}
