package park

import (
	stderrors "errors"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/genproto/googleapis/cloud/dataproc/v1"
)

const (
	MaxChannels  = 256
	MaxFunctions = 64
	MaxListeners = 256
)

var (
	ErrLimited error = stderrors.New("Exceeds limit!")
	ErrState   error = stderrors.New("Invalid state!")
	ErrNotReady error = stderrors.New("Not ready!")
)

// TourEvent passes to the listeners on the status change of Tours
type TourEvent[T any] struct{}

// RoutineStatus indicates the status of a go routine
type RoutineStatus int

const (
	RsInit RoutineStatus = iota + 1	// Go routine created
	RsReady	// Routine is ready for work
	RsClosing	// Routine is about to close, it's read only
	RsClosed	// Routine is closed
)

type ReType int

const (
	ReChannel ReType = iota + 1
	ReFunc
)

// RoutineEvent notifies the status of channel and their functions
type RoutineEvent struct{
	typ 	ReType
	cid int
	fid int
	status RoutineStatus
}

// Park Configurations
type ParkConf[T any] struct {
	numChans  int                // specify the number of channels
	funcs     []func(T) error   // functions to be called in each channel
	listeners []func(*TourEvent[T]) // listeners who concerns with tour status change
}

// A function to set a single configuration item
type Optf[T any] func(*ParkConf[T]) error

// Create a new `ParkConf`
func NewParkConf[T any](optfs ...Optf[T]) (*ParkConf[T], error) {
	// build a default options
	conf := &ParkConf[T]{
		numChans:  1,
		funcs:     make([]func(T) error, 4),
		listeners: make([]func(*TourEvent[T]), 2),
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
func WithNumChannels[T any](v int) Optf[T] {
	return func(conf *ParkConf[T]) error {
		if v > MaxChannels || v < 0 {
			return ErrLimited
		}
		conf.numChans = v
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
func WithListeners[T any](ls *[]func(*TourEvent[T])) Optf[T] {
	return func(conf *ParkConf[T]) error {
		if len(*ls) > MaxListeners {
			return ErrLimited
		}
		conf.listeners = ls
		return nil
	}
}

// get the number of channels
func (opts ParkConf[T]) NumChannels() int {
	return opts.numChans
}

/*
The park!
*/
type Park[T any] struct {
	conf     *ParkConf[T]       // configurations
	dc 	chan T	// data channel
	sc 	chan Tour[T]	// successful channel
	fc 	chan Tour[T]	// failure channel
	rec 	chan RoutineEvent 
	tec	chan TourEvent	//
	status   Status             // status
	total    int                // total tourists, -1 means unpredicatable
	succeeds int32              // number of tourists who complete all of their tours successfully
	failures int32              // number of tourists who encount failure
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

// The Tour hold the infomation about the tourist
type Tour[T any] struct {
	err *error
	t   *T
}

// a singleton instance of park, the generic type is any
var t *Park[any]

func init() {
	t = New[any]()
}

// park creation method for multi instance mode
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

// set the total tourists to be served, -1 means unpredicatable
func (t *Park[T]) SetTotal(total int) error {
	if t.status != ParkInitial {
		return ErrState
	}
	t.total = total
	return nil
}

// get the number of finished tourists
func Finished() int {
	return t.Finished()
}

// get the number of finished tourists
func (t *Park[T]) Finished() int {
	return int(atomic.LoadInt32(&(t.succeeds)))
}

// get the status of park
func GetStatus() Status {
	return t.Status()
}

// get the status of park
func (t *Park[T]) Status() Status {
	return t.status
}

// get the configuration of park
func Conf() *ParkConf[any] {
	return t.Conf()
}

// get the configuration of park
func (t *Park[T]) Conf() *ParkConf[T] {
	return t.conf
}

// set the configuration of park
func SetConf(options *ParkConf[any]) error {
	return t.SetConf(options)
}

// set the configuration of park
func (t *Park[T]) SetConf(options *ParkConf[T]) error {
	if t.status != ParkInitial {
		return errors.WithMessagef(ErrState, 
			"desired is Initial, actual : %d", t.status)
	}
	t.conf = options
	return nil
}

// cancel all ongoing tour
func Cancel() error {
	return t.Cancel()
}

// cancel all ongoing tour
func (t *Park[T]) Cancel() error {
	t.status = ParkAborted
	return nil
}

// reset the runtime information of the park
func Reset() error {
	return t.Reset()
}

// reset the runtime information of the park
func (t *Park[T]) Reset() error {
	if t.status == ParkPaused || t.status == ParkOpen {
		err := t.Cancel()
		if err != nil {
			return errors.Wrap(err, "cancel")
		}
	} else {
		if t.status == ParkClosed {
			return errors.Wrap(ErrState, "can not reset park when it's closed")
		}
	}
	t.status = ParkInitial
	t.total = -1
	t.succeeds = 0
	return nil
}

// start running
func Start(dataq <-chan any) (successq <-chan any, errorq <-chan any, err error) {
	return t.Start()
}

// start running
func (t *Park[T]) Start(dataq <-chan T) (successq <-chan T, errorq <-chan T, err error) {
	t.conf.dc = dataq
	t.conf.sc = make(chan Tour[T], 8)
	t.conf.fc = make(chan Tour[T], 8)
	eventq := make(chan RoutineEvent, 8)
	chls := t.conf.numChans	
	for i := 0; i < chls; i++ {
		t.newChannel(i)
	}
	cnt := 0
	stopLooping := false 

	for {
		select {
		case e, ok := <-eventq:
			if ok {
				if e.typ == ReChannel && e.status == RsReady {
					cnt++
				}
				if cnt >= chls {
					stopLooping = true 
				}
			} else {
				stopLooping = true 
			}
		}
		if stopLooping {
			break
		}
	}
	if cnt < chls {
		return nil, nil, errors.WithMessagef(ErrNotReady, "Ready channels %d, desired %d", cnt, chls)
	}
	return t.conf.sc, t.conf.fc, nil
}

type uow[T any] struct {
	queue chan *Tour[T]
	quit  chan *Tour[T]
}

func (p *Park[T]) start(f func( data chan<- T) err error,	next chan<- T, errc ->chan T) *uow[T] {
	queue := make(chan *Tour[T], 10)
	quit := make(chan *Tour[T])

	l := len(*p.conf.funcs) - 1

	go func() {
		for {
			select {
			case <-quit:
				return
			case v := <-queue:
				err := f(v.t)
				if err != nil {
					v.err = &err
					v -> errc
				}
				if id >= l {
					atomic.AddInt32(&(p.succeeds), 1)
					for _, listener := range *p.conf.listeners {
						// TODO : compose the event when tour finished
						listener(&TourEvent{})
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
		l := len(p.conf.numChans)
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
