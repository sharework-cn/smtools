package park

import (
	stderrors "errors"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

const (
	MaxChannels  = 256
	MaxFunctions = 64
	MaxListeners = 256
)

var (
	ErrLimited error = stderrors.New("Exceeds limit!")
	ErrState   error = stderrors.New("Invalid state!")
)

// Event passes to the listeners on the status change of Tours
type Event[T any] struct{}

// Park Configurations
type ParkConf[T any] struct {
	channels  int                // specify the number of channels
	funcs     *[]func(T) error   // functions to be called in each channel
	listeners *[]func(*Event[T]) // listeners who concerns with tour status change
}

// A function to set a single configuration item
type Optf[T any] func(*ParkConf[T]) error

// Create a new `ParkConf`
func NewParkConf[T any](optfs ...Optf[T]) (*ParkConf[T], error) {
	// build a default options
	conf := &ParkConf[T]{
		channels:  1,
		funcs:     nil,
		listeners: nil,
	}
	// accept custom options
	for _, optf := range optfs {
		err := optf(conf)
		if err != nil {
			return nil, errors.Wrap(err, "validate")
		}
	}
	return conf, nil
}

// Return a function to set channels
func WithChannels[T any](v int) Optf[T] {
	return func(conf *ParkConf[T]) error {
		if v > MaxChannels || v < 0 {
			return ErrLimited
		}
		conf.channels = v
		return nil
	}
}

// return a function to set functions
func WithFuncs[T any](t *[]func(T) (err error)) Optf[T] {
	return func(conf *ParkConf[T]) error {
		if len(*t) > MaxFunctions {
			return ErrLimited
		}
		conf.funcs = t
		return nil
	}
}

// return a function to set listeners
func WithListeners[T any](ls *[]func(*Event[T])) Optf[T] {
	return func(conf *ParkConf[T]) error {
		if len(*ls) > MaxListeners {
			return ErrLimited
		}
		conf.listeners = ls
		return nil
	}
}

// get the number of channels
func (opts *ParkConf[T]) Channels() int {
	return opts.channels
}

/*
The park!
*/
type Park[T any] struct {
	conf     *ParkConf[T]       // configurations
	status   Status             // status
	total    int                // total tourists to be served, -1 means unpredicatable
	finished int32              // number of tourists had been served
	luid     int32              // last uid
	ongoing  map[int32]*Tour[T] // the tours of ongoing tourists
}

// Status of park
type Status int

const (
	ParkInitial Status = iota + 1 // newly created
	ParkOpen                      // it's open, the configuration can not be modified
	ParkPaused                    // paused, no new tourist allowed, can be resumed
	ParkClosed                    // closed, can not be resumed
	ParkAborted                   // canceled
)

// The workload can be consumed by the worker
type Tour[T any] struct {
	uid int32
	err *error
	t   *T
}

// a singleton instance of park, the generic type is any
var t *Park[any]

func init() {
	t = New[any]()
}

// multi instance mode
func New[T any]() *Park[T] {
	park := new(Park[T])
	park.conf, _ = NewParkConf[T]()
	park.Reset()
	return park
}

// get the total tourists to be served, -1 means unpredicatable
func Total() int {
	return t.Total()
}

// get the total tourists to be served, -1 means unpredicatable
func (t *Park[T]) Total() int {
	return t.total
}

// set the total tourists to be served, -1 means unpredicatable
func SetTotal(total int) error {
	return t.SetTotal(total)
}

func (t *Park[T]) SetTotal(total int) error {
	if t.status != ParkInitial {
		return ErrState
	}
	t.total = total
	return nil
}

func Finished() int {
	return t.Finished()
}

func (t *Park[T]) Finished() int {
	return int(atomic.LoadInt32(&(t.finished)))
}

func GetStatus() Status {
	return t.Status()
}

func (t *Park[T]) Status() Status {
	return t.status
}

func Conf() *ParkConf[any] {
	return t.Conf()
}

func (t *Park[T]) Conf() *ParkConf[T] {
	return t.conf
}

func SetConf(options *ParkConf[any]) error {
	return t.SetConf(options)
}

func (t *Park[T]) SetConf(options *ParkConf[T]) error {
	if t.status != ParkInitial {
		return errors.New("Invalid State!")
	}
	t.conf = options
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
	t.luid = 0
	t.ongoing = make(map[int32]*Tour[T], 16)
	return nil
}

func Start(data <-chan any) error {
	return t.Start(data)
}

func (t *Park[T]) Start(data <-chan T) error {
	for i := 0; i < t.options.concurrency; i++ {
		t.newChannel(i, data)
	}
	time.Sleep(3 * time.Second)
	return nil
}

type uow[T comparable] struct {
	queue chan *Tour[T]
	quit  chan *Tour[T]
}

func (p *Park[T]) start(id int, f func(*Context[T], *Tour[T]) error,
	next chan<- *Tour[T]) *uow[T] {
	queue := make(chan *Tour[T], 10)
	quit := make(chan *Tour[T])

	l := len(*p.options.channelFuncs) - 1

	go func() {
		for {
			select {
			case <-quit:
				return
			case v := <-queue:
				if !(*v).HasError() {
					err := f(&Context[T]{
						channelId: id,
						options:   p.options,
					}, v)
					if err != nil {
						(*v).AddError(err)
					}
				}
				if id >= l {
					atomic.AddInt32(&(p.finished), 1)
					for _, listener := range *p.options.listeners {
						listener(v, p.Finished(), p.total)
					}
				}
				if next != nil {
					next <- v
				}
			}
		}
	}()
	return &uow[T]{queue: queue, quit: quit}
}

func (p *Park[T]) newChannel(id int, queue <-chan *Tour[T]) {
	go func(id int) {
		l := len(*p.options.channelFuncs)
		uows := make([]*uow[T], l)
		for i := l - 1; i >= 0; i-- {
			if i == l-1 {
				uows[i] = p.start(id, (*p.options.channelFuncs)[i], nil)
			} else {
				uows[i] = p.start(id, (*p.options.channelFuncs)[i], (*uows[i+1]).queue)
			}
		}

		for {
			select {
			case v, ok := <-queue:
				if !ok {
					for _, u := range uows {
						close(u.quit)
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
