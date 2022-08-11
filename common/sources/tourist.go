package tourist

import (
	"errors"
)

// Tourist Options
type TouristOptions struct {
	concurrency    int // specify the number of workers
	timeoutMinutes int // timeout in minutes
}

// A setter for Single Option
type TouristOption func(*TouristOptions)

// Create a new `TouristOptions`
func NewTouristOptions(options ...TouristOption) *TouristOptions {
	// build a default options
	opts := &TouristOptions{
		concurrency:    1,
		timeoutMinutes: 5,
	}
	// accept custom options
	for _, option := range options {
		option(opts)
	}
	return opts
}

// TouristOptions - Set Concurrency
func WithConcurrency(v int) TouristOption {
	return func(opts *TouristOptions) {
		opts.concurrency = v
	}
}

// TouristOptions - Set Timeout in Minutes
func WithTimoutMinutes(v int) TouristOption {
	return func(opts *TouristOptions) {
		opts.timeoutMinutes = v
	}
}

func (opts *TouristOptions) Concurrency() int {
	return opts.concurrency
}

func (opts *TouristOptions) TimeoutMinutes() int {
	return opts.timeoutMinutes
}

// Status of workload
type Status int

const (
	TouristInitial Status = iota + 1
	TouristCheckedIn
	TouristStarted
	TouristPaused
	TouristFinished
	TouristCanceled
)

// Status of tour
type TourStatus int

const (
	TourPending TourStatus = iota + 1
	TourInProgress
	TourCompleted
	TourError
	TourCanceled
)

// The workload can be consumed by the worker
type Tour struct {
	name   string     // file name
	status TourStatus //
}

type Context struct {
	options *TouristOptions
}

var t *Tourist

func init() {
	t = New()
}

func New() *Tourist {
	tourist := new(Tourist)
	tourist.options = NewTouristOptions()
	tourist.Reset()
	return tourist
}

func Total() int {
	return t.Total()
}

func (t *Tourist) Total() int {
	return t.total
}

func Finished() int {
	return t.Finished()
}

func (t *Tourist) Finished() int {
	return t.finished
}

func GetStatus() Status {
	return t.Status()
}

func (t *Tourist) Status() Status {
	return t.status
}

func Options() *TouristOptions {
	return t.Options()
}

func (t *Tourist) Options() *TouristOptions {
	return t.options
}

func SetOptions(options *TouristOptions) error {
	return t.SetOptions(options)
}

func (t *Tourist) SetOptions(options *TouristOptions) error {
	if t.status != TouristInitial {
		return errors.New("Invalid State!")
	}
	t.options = options
	return nil
}

type Tourist struct {
	options   *TouristOptions
	entrance  string
	status    Status
	total     int
	finished  int
	workloads map[string]*Tour
}

type Visitor interface {
	Visit(ctx Context, t Tour) (err error)
}

type Listener interface {
	OnNoticed(t Tour, finished int, total int)
}

type Checker interface {
	Check(name string) ([]string, error)
}

func Cancel() error {
	return t.Cancel()
}

func (t *Tourist) Cancel() error {
	return nil
}

func Reset() error {
	return t.Reset()
}

func (t *Tourist) Reset() error {
	if t.status == TouristPaused || t.status == TouristStarted {
		err := t.Cancel()
		if err != nil {
			return err
		}
	}
	t.entrance = ""
	t.status = TouristInitial
	t.total = 0
	t.finished = 0
	t.workloads = make(map[string]*Tour, 16)
	return nil
}

func Enter(entrance string, checker Checker) error {
	return t.Enter(entrance, checker)
}

func (t *Tourist) Enter(entrance string, checker Checker) error {
	err := Reset()
	if err != nil {
		return err
	}
	sc, err := checker.Check(entrance)
	if err != nil {
		return err
	}
	for _, s := range sc {
		tr := new(Tour)
		tr.name = s
		tr.status = TourPending
		t.workloads[s] = tr
	}
	t.status = TouristCheckedIn
	return nil
}

func Start() error {
	return t.Start()
}

func (t *Tourist) Start() error {
	return nil
}
