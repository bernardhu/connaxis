package pool

import (
	"sync"

	"github.com/bernardhu/connaxis/wrapper"
)

var GAlloctor *BytesAlloctor
var once sync.Once

type BytesAlloctor struct {
	step       int
	rank       int
	max        int
	allocorMap sync.Map
}

type ObjectPool[T any] struct {
	p sync.Pool
}

func NewObjectPool[T any](newFn func() *T) *ObjectPool[T] {
	p := &ObjectPool[T]{}
	p.p.New = func() any {
		return newFn()
	}
	return p
}

func (p *ObjectPool[T]) Get() *T {
	v := p.p.Get()
	if v == nil {
		return nil
	}
	return v.(*T)
}

func (p *ObjectPool[T]) Put(v *T) {
	if v == nil {
		return
	}
	p.p.Put(v)
}

func Setup(rank int) {
	once.Do(
		func() {
			GAlloctor = new(BytesAlloctor)
			GAlloctor.step = 1024
			GAlloctor.rank = rank
			GAlloctor.max = 0
			GAlloctor.initialize()
		},
	)
}

func (ba *BytesAlloctor) initialize() {
	if ba.step&(ba.step-1) != 0 {
		wrapper.Fatalf("BytesAlloctor step shoud be the power of 2")
	}

	for i := 0; i < ba.rank; i++ {
		size := (1 << i) * ba.step
		wrapper.Debugf("add pool for %d", size)
		ba.allocorMap.Store(size, &sync.Pool{
			New: func() interface{} {
				return make([]byte, size)
			},
		})
		ba.max = size
	}
}

func (ba *BytesAlloctor) Get(size int) []byte {
	if size > ba.max {
		return nil
	}

	target := ba.step
	if size > ba.step {
		if size&(size-1) == 0 {
			target = size
		} else {
			for i := 0; i < ba.rank; i++ {
				target = (1 << i) * ba.step
				if target >= size {
					break
				}
			}
		}
	}

	item, _ := ba.allocorMap.Load(target)
	pool := item.(*sync.Pool)
	return pool.Get().([]byte)

}

func (ba *BytesAlloctor) Put(buf []byte) {
	size := len(buf)
	if (size > ba.max && size < ba.step) || (size&(size-1) != 0) || size == 0 {
		return
	}

	item, ok := ba.allocorMap.Load(size)
	if ok == false {
		return
	}
	pool := item.(*sync.Pool)
	//nolint:staticcheck // SA6002: keep []byte directly in sync.Pool for simpler byte-slice allocator semantics.
	pool.Put(buf)
}
