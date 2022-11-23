package park

import (
	stderrors "errors"
	"runtime"
	"sync/atomic"
	"time"
	"log"
	"github.com/pkg/errors"
	"google.golang.org/genproto/googleapis/cloud/dataproc/v1"
)

const (
	MaxChannels  = 256
	MaxFunctions = 64
	MaxListeners = 256
	ChannelCache = 8
)

var (
	ErrArg         = stderrors.New("invalid argument")
	ErrState       = stderrors.New("invalid state")
//	ErrOp    = stderrors.New("invalid operation")
	ErrAbort = stderrors.New("abort anyway")
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

// GetConf Park Configurations
type Conf[T any] struct {
	numChans  int                // specify the number of channels
	funcs     []func(T) error   // functions to be called in each channel
	listeners []func(*TourEvent[T]) // listeners who concerns with tour status change
}

// Optf An option function defines the way to set a configuration option
type Optf[T any] func(*Conf[T]) error

// NewParkConf Create a new `GetConf`
func NewParkConf[T any](optfs ...Optf[T]) (*Conf[T], error) {
	// build a default options
	conf := &Conf[T]{
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

// WithNumChannels Return a function to set channels
func WithNumChannels[T any](v int) Optf[T] {
	return func(conf *Conf[T]) error {
		if v > MaxChannels || v < 0 {
			return ErrArg
		}
		conf.numChans = v
		return nil
	}
}

// WithFuncs return a function to set functions
func WithFuncs[T any](t []func(T) error) Optf[T] {
	return func(conf *Conf[T]) error {
		if len(t) > MaxFunctions {
			return ErrArg
		}
		conf.funcs = t
		return nil
	}
}

// WithListeners return a function to set listeners
func WithListeners[T any](ls []func(*TourEvent[T])) Optf[T] {
	return func(conf *Conf[T]) error {
		if len(ls) > MaxListeners {
			return ErrArg
		}
		conf.listeners = ls
		return nil
	}
}

// NumChannels get the number of channels
func (opts Conf[T]) NumChannels() int {
	return opts.numChans
}

// Park The Park container
type Park[T any] struct {
	conf     *Conf[T]      // configurations
	es 		*EventServer[T] // event server
	dq 	<-chan T            // data queue which provided by client
	dsq 	chan Tour[T]       // dispatching queue
	ppq chan Tour[T]          // post processing queue
	endq chan struct{}        // ending queue
	status   Status           // status
}

// Status of park
type Status int

const (
	StateInitial Status = iota + 1 // newly created
	Open                           // it's open, the configuration can not be modified
	ParkPaused                     // paused, no new tourist allowed, can be resumed
	ParkClosed                     // closed, can not be resumed
	ParkAborted                    // canceled
)

/*
Event Server!
*/
type EventServer[T any] struct {
	listeners []func(*TourEvent[T])	// listeners
	eq chan TourEvent[T] // event queue
	eqx chan struct{} // exiting queue for the looping on the eq
	eeq chan struct{}	// ending queue for the eq
}

// The Tour hold the information about the tourist
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

// get the status of park
func GetStatus() Status {
	return t.Status()
}

// get the status of park
func (t *Park[T]) Status() Status {
	return t.status
}

// get the configuration of park
func GetConf() *Conf[any] {
	return t.GetConf()
}

// get the configuration of park
func (t *Park[T]) GetConf() *Conf[T] {
	return t.conf
}

// set the configuration of park
func SetConf(options *GetConf[any]) error {
	return t.SetConf(options)
}

// set the configuration of park
func (t *Park[T]) SetConf(options *GetConf[T]) error {
	if t.status != StateInitial {
		return errors.WithMessagef(ErrState, 
			"Can not configurate in state : %d", t.status)
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
	if t.status == ParkPaused || t.status == Open {
		err := t.Cancel()
		if err != nil {
			return errors.Wrap(err, "cancel")
		}
	} else {
		if t.status == ParkClosed {
			return errors.Wrap(ErrState, "Can not reset the park when it's closed")
		}
	}
	t.status = StateInitial
	return nil
}



// start running
func Start(data <-chan any) (err error) {
	return t.Start()
}

// start running
func (t *Park[T]) Start(data <-chan any) (err error) {
	t.dq = data
	t.dsq = make(chan Tour[T], ChannelCache)
	t.ppq = make(chan Tour[T], ChannelCache)
	t.eq = make(chan TourEvent, ChannelCache)
	t.endq = make(chan struct{})
	t.eeq = make(chan struct{})
	
	chls := t.conf.numChans	
	for i := 0; i < chls; i++ {
		t.newChannel(i, data)
	}
	return nil
}

func (t *Park[T]) startEventListener() {
	t.dq = data
	t.dsq = make(chan Tour[T], ChannelCache)
	t.ppq = make(chan Tour[T], ChannelCache)
	t.eq = make(chan TourEvent, ChannelCache)
	t.endq = make(chan struct{})
	t.eeq = make(chan struct{})
	
	chls := t.conf.numChans	
	for i := 0; i < chls; i++ {
		t.newChannel(i, data)
	}
	return nil
}

/*
func (t *Park[T]) waitForChannelsReady() (err error) {
	cnt := 0
	stopLooping := false 
	chls := t.conf.numChans
	for {
		select {
		case e, ok := <-t.rec:
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
		return errors.WithMessagef(ErrOp, "Ready channels %d, desired %d", cnt, chls)
	}
	return nil
}
*/

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



// create a new event server
func (t *Park[T])newEventServer() {
	t.es = &EventServer[T]{
		listeners: t.conf.listeners,
		eq: make( chan TourEvent[T], ChannelCache),
		eqx: make( chan struct{}),
		eeq: make( chan struct{}),
	}
}

func (es *EventServer[T])start() {
	defer func() {
		if err := recover(); err != nil && err != ErrAbort {
			
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			c.server.logf("http: panic serving %v: %v\n%s", c.remoteAddr, err, buf)
		}
		if inFlightResponse != nil {
			inFlightResponse.cancelCtx()
		}
		if !c.hijacked() {
			if inFlightResponse != nil {
				inFlightResponse.conn.r.abortPendingRead()
				inFlightResponse.reqBody.Close()
			}
			c.close()
			c.setState(c.rwc, StateClosed, runHooks)
		}
	}()

	for {
		select {
		case <-es.eqx:
			break
		case e, ok := <-es.eq:
			for i, l := range es.listeners {
				
				l(*e)
			}
		}
	}
}