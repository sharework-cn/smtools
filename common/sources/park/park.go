package park

import (
	"time"

	"github.com/pkg/errors"
)

// T any - the ticket identifier
type Park[T comparable] struct {
	options  *ParkOptions[T]
	status   Status
	total    int
	finished int32
}

// Status of workload
type Status int

const (
	ParkInitial Status = iota + 1
	ParkOpen
	ParkPaused
	ParkClosed
	ParkAborted
)

// The workload can be consumed by the worker
type Tour[T comparable] interface {
	GetTicket() T
}

type Context[T comparable] struct {
	channelId int
	options   *ParkOptions[T]
}

var t *Park[string]

func init() {
	t = New[string]()
}

func New[T comparable]() *Park[T] {
	tourist := new(Park[T])
	tourist.options = NewTouristOptions[T]()
	tourist.Reset()
	return tourist
}

func Total() int {
	return t.Total()
}

func (t *Park[T]) Total() int {
	return t.total
}

func SetTotal(total int) error {
	return t.SetTotal(total)
}

func (t *Park[T]) SetTotal(total int) error {
	if t.status != ParkInitial {
		return errors.New("Invalid State!")
	}
	t.total = total
	return nil
}

func Finished() int {
	return t.Finished()
}

func (t *Park[T]) Finished() int {
	return int(t.finished)
}

func GetStatus() Status {
	return t.Status()
}

func (t *Park[T]) Status() Status {
	return t.status
}

func Options() *ParkOptions[string] {
	return t.Options()
}

func (t *Park[T]) Options() *ParkOptions[T] {
	return t.options
}

func SetOptions(options *ParkOptions[string]) error {
	return t.SetOptions(options)
}

func (t *Park[T]) SetOptions(options *ParkOptions[T]) error {
	if t.status != ParkInitial {
		return errors.New("Invalid State!")
	}
	t.options = options
	return nil
}

func Cancel() error {
	return t.Cancel()
}

func (t *Park[T]) Cancel() error {
	return nil
}

func Reset() error {
	return t.Reset()
}

func (t *Park[T]) Reset() error {
	if t.status == ParkPaused || t.status == ParkOpen {
		err := t.Cancel()
		if err != nil {
			return errors.Wrap(err, "cancel")
		}
	}
	t.status = ParkInitial
	t.total = -1
	t.finished = 0
	return nil
}

func Start(data chan *Tour[string]) error {
	return t.Start(data)
}

func (t *Park[T]) Start(data chan *Tour[T]) error {
	for i := 0; i < t.options.concurrency; i ++{
		t.newChannel(i, data)
	}
	time.Sleep(3 * time.Second)
	return nil
}

type uow[T comparable] struct {
	queue  chan *Tour[T]
	quit   chan *Tour[T]
	result chan *Tour[T]
}

func (p *Park[T]) start(id int, f func(*Context[T], *Tour[T]) error,
	next chan<- *Tour[T]) *uow[T] {
	queue := make(chan *Tour[T], 10)
	quit := make(chan *Tour[T])
	result := make(chan *Tour[T])

	go func() {
		for {
			select {
			case v := <-quit:
				result <- v
				return
			case v := <-queue:
				err := f(&Context[T]{
					channelId: id,
					options:   p.options,
				}, v)
				if err != nil {
					// todo : when error occurs
				} else {
					if next != nil {
						next <- v
					}
				}
			}
		}
	}()
	return &uow[T]{queue: queue, quit: quit, result: result}
}

func (p *Park[T]) newChannel(id int, queue <-chan *Tour[T]) {
	go func(id int) {
		l := len(*p.options.channelTemplate)
		uows := make([]*uow[T], l)
		for i := l - 1; i >= 0; i-- {
			if i == l-1 {
				uows[i] = p.start(id, (*p.options.channelTemplate)[i], nil)
			} else {
				uows[i] = p.start(id, (*p.options.channelTemplate)[i], (*uows[i+1]).queue)
			}
		}

		for {
			select {
			case v, ok := <-queue:
				if !ok {
					for _, q := range uows {
						close(q.quit)
					}
					return
				}
				if l > 0 {
					(*uows[0]).queue <- v
				}
			}
		}
	}(id)
}
