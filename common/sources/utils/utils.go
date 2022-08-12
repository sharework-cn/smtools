package utils

import (
	"errors"
	"sync/atomic"
)

var (
	EOF = errors.New("EOF")
)

func Zero[T any]() T {
	var z T
	return z
}

func IsZero[T comparable](v T) bool {
	return v == *new(T)
}

func IsEof(err error) bool {
	return err == EOF
}

type Iterator[E any] interface {
	Next() (E, error)
}

type Fixed[E any] interface {
	Size() int
}

type FixedIterator[E any] interface {
	Iterator[E]
	Fixed[E]
}

type arrayIterator[E any] struct {
	idx  *int32
	data *[]E
}

func (e *arrayIterator[E]) Size() int {
	return len(*e.data)
}

func (e *arrayIterator[E]) Next() (E, error) {
	atomic.AddInt32(e.idx, 1)
	x := atomic.LoadInt32(e.idx)
	if int(x) >= len(*(e.data)) {
		return Zero[E](), EOF
	}
	return (*e.data)[x], nil
}

func NewArrayIterator[E any](data *[]E) FixedIterator[E] {
	var i int32 = -1
	return &arrayIterator[E]{
		idx:  &i,
		data: data,
	}
}
