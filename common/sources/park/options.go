package park

// Park Options
type ParkOptions[T comparable] struct {
	concurrency     int // specify the number of channels
	timeoutMinutes  int // timeout in minutes
	channelTemplate *[]func(ctx *Context[T], t *Tour[T]) (err error)
	listeners       *[]func(t *Tour[T], finished int, total int)
}

// A setter for Single Option
type ParkOption[T comparable] func(*ParkOptions[T])

// Create a new `TouristOptions`
func NewTouristOptions[T comparable](options ...ParkOption[T]) *ParkOptions[T] {
	// build a default options
	opts := &ParkOptions[T]{
		concurrency:     1,
		timeoutMinutes:  5,
		channelTemplate: nil,
		listeners:       nil,
	}
	// accept custom options
	for _, option := range options {
		option(opts)
	}
	return opts
}

// TouristOptions - Set Concurrency
func WithConcurrency[T comparable](v int) ParkOption[T] {
	return func(opts *ParkOptions[T]) {
		opts.concurrency = v
	}
}

// TouristOptions - Set Timeout in Minutes
func WithTimoutMinutes[T comparable](v int) ParkOption[T] {
	return func(opts *ParkOptions[T]) {
		opts.timeoutMinutes = v
	}
}

// TouristOptions - Set Channel template
func WithChannelTemplate[T comparable](t *[]func(ctx *Context[T],
	t *Tour[T]) (err error)) ParkOption[T] {
	return func(opts *ParkOptions[T]) {
		opts.channelTemplate = t
	}
}

// TouristOptions - Set Channel template
func WithListeners[T comparable](ls *[]func(t *Tour[T],
	finished int, total int)) ParkOption[T] {
	return func(opts *ParkOptions[T]) {
		opts.listeners = ls
	}
}

func (opts *ParkOptions[T]) Concurrency() int {
	return opts.concurrency
}

func (opts *ParkOptions[T]) TimeoutMinutes() int {
	return opts.timeoutMinutes
}

func (opts *ParkOptions[T]) Valid() bool {
	return true
}
