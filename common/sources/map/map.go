package fmap

import "errors"

const (
	MaxFunctions int = 1024
)

var ErrOverflowed = errors.New("Function Map Overflowed!")

type Ftype int16

const (
	Func Ftype = iota + 1
	Supplier
	Consumer
	Collector
	ObjFunc
	ResultFunc
)

type fidx struct {
	t Ftype
	i int16
}

type Fmap[T any, R any] struct {
	funcs      []func(T) (R, error)
	suppliers  []func() (T, error)
	consumers  []func(T) error
	collectors []func(T, R) (T, error)
	ofs        []func(T) (T, error)
	rfs        []func(R) (R, error)
	idxs       []fidx
}

func (m *Fmap[T, R]) Map(f func(T) (R, error)) error {
	if len(m.idxs) >= MaxFunctions {
		return ErrOverflowed
	}
	m.idxs = append(m.idxs, fidx{Func, int16(len(m.funcs))})
	m.funcs = append(m.funcs, f)
	return nil
}

func (m *Fmap[T, R]) Supply(f func() (T, error)) error {
	if len(m.idxs) >= MaxFunctions {
		return ErrOverflowed
	}
	m.idxs = append(m.idxs, fidx{Supplier, int16(len(m.suppliers))})
	m.suppliers = append(m.suppliers, f)
	return nil
}

func (m *Fmap[T, R]) Consume(f func(T) error) error {
	if len(m.idxs) >= MaxFunctions {
		return ErrOverflowed
	}
	m.idxs = append(m.idxs, fidx{Consumer, int16(len(m.consumers))})
	m.consumers = append(m.consumers, f)
	return nil
}

func (m *Fmap[T, R]) Collect(f func(T, R) (T, error)) error {
	if len(m.idxs) >= MaxFunctions {
		return ErrOverflowed
	}
	m.idxs = append(m.idxs, fidx{Collector, int16(len(m.collectors))})
	m.collectors = append(m.collectors, f)
	return nil
}

func (m *Fmap[T, R]) ObjOp(f func(T) (T, error)) error {
	if len(m.idxs) >= MaxFunctions {
		return ErrOverflowed
	}
	m.idxs = append(m.idxs, fidx{ObjFunc, int16(len(m.ofs))})
	m.ofs = append(m.ofs, f)
	return nil
}

func (m *Fmap[T, R]) ResultOp(f func(R) (R, error)) error {
	if len(m.idxs) >= MaxFunctions {
		return ErrOverflowed
	}
	m.idxs = append(m.idxs, fidx{ObjFunc, int16(len(m.rfs))})
	m.rfs = append(m.rfs, f)
	return nil
}
