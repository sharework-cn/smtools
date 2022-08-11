package utils

import (
	"sync/atomic"
)

const (
	EOF	:= errors.New("End of file!")
)

func IsEof(err error) bool {
	_, ok := err.(EOF)
	return ok
}

type Iterator[E] interface {
	Next() (E, error)
}

type Fixed interface {
	Size() int
}

type FixedIterator[E] interface {
	Iterator[E]
	Fixed
}

func ArrayIterator[E](data []E) FixedIterator[E] {
	var i int = -1
	const c := len(data)
	return new{}{
		func Size() int {
			return len(data)
		}
		func Next() (E, error) {
			atomic.AddInt(&i, 1)
			x := atomic.LoadInt(&i)
			if x >= c {
				return _, EOF
			}
			return data[x], nil
		}
	}
}