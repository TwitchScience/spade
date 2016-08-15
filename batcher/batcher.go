package batcher

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/twitchscience/aws_utils/logger"
)

// Config is used to configure a batcher instance
type Config struct {
	// MaxSize is the max combined size of the batch
	MaxSize int

	// MaxAge is the max age of the oldest entry in the glob
	MaxAge string

	// BufferLength is the length of the channel where newly
	// submitted entries are stored, decreasing the size of this
	// buffer can cause stalls, and increasing the size can increase
	// shutdown time
	BufferLength int
}

func (c *Config) Validate() error {
	maxAge, err := time.ParseDuration(c.MaxAge)
	if err != nil {
		return err
	}

	if maxAge <= 0 {
		return errors.New("MaxAge must be a positive value")
	}

	if c.MaxSize <= 0 {
		return errors.New("MaxSize must be a positive value")
	}

	if c.BufferLength == 0 {
		return errors.New("BufferLength must be a positive value")
	}

	return nil
}

// Complete is the type of a function that Batcher will
// call for every completed batch
type Complete func([][]byte)

// A Batcher will batch togther a slice of byte slices, based
// on a size and timer criteria
type Batcher struct {
	config      Config
	completor   Complete
	incoming    chan []byte
	pending     [][]byte
	pendingSize int
	timer       *time.Timer
	maxAge      time.Duration

	sync.WaitGroup
}

// New returns a newly created instance of Batcher
func New(config Config, completor Complete) (*Batcher, error) {
	err := config.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid config: %s", err)
	}
	maxAge, _ := time.ParseDuration(config.MaxAge)

	b := &Batcher{
		config:    config,
		completor: completor,
		maxAge:    maxAge,
		timer:     time.NewTimer(maxAge),
		incoming:  make(chan []byte, config.BufferLength),
	}

	b.Add(1)
	logger.Go(b.worker)
	return b, nil
}

// Submit submits an object to be batched
func (b *Batcher) Submit(entry []byte) {
	b.incoming <- entry
}

// Close closes the batcher. Will return after all
// entries are flushed
func (b *Batcher) Close() {
	close(b.incoming)
	b.Wait()
}

func (b *Batcher) add(entry []byte) {
	s := len(entry) + b.pendingSize
	if s > b.config.MaxSize {
		b.complete()
	}

	if len(b.pending) == 0 {
		b.timer.Reset(b.maxAge)
	}

	b.pending = append(b.pending, entry)
	b.pendingSize += len(entry)

}

func (b *Batcher) complete() {
	if len(b.pending) == 0 {
		return
	}

	b.completor(b.pending)
	b.pending = nil
	b.pendingSize = 0
}

func (b *Batcher) worker() {
	defer b.Done()
	defer b.complete()
	for {
		select {
		case <-b.timer.C:
			b.complete()
		case e, ok := <-b.incoming:
			if !ok {
				return
			}
			b.add(e)
		}
	}
}
