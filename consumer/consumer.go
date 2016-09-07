package consumer

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/cactus/go-statsd-client/statsd"
	"github.com/twitchscience/aws_utils/logger"
	"github.com/twitchscience/kinsumer"
	kstatsd "github.com/twitchscience/kinsumer/statsd"
)

// Config is used to set configuration variables for the Consumer
type Config struct {
	// ApplicationName is the name that kinsumer uses to communicate with other clients
	// consuming the same kinesis stream
	ApplicationName string

	// StreamName is the name of the stream that is being consumed from
	StreamName string

	// (Optional) Time for Kinsumer to sleep if there are no new records
	ThrottleDelay string

	// (Optional) Delay before the checkpoint for each shard is committed to the database
	CommitFrequency string

	// (Optional) How frequently the list of shards are checked
	ShardCheckFrequency string

	// (Optional) Size of the internal buffer for kinesis events
	BufferSize int
}

// Result is the next data/error to be consumed from the kinsumer or standard input.
type Result struct {
	Data  []byte
	Error error
}

// ResultPipe consumes input from somewhere and provides Results through its ReadChannel.
type ResultPipe interface {
	// ReadChannel provides a channel from which the Results are read.
	ReadChannel() <-chan *Result

	// Close cleans up any resources associated with the pipe.
	Close()
}

// StandardInputPipe is a ResultPipe that consumes plaintext events from standard input.
type StandardInputPipe struct {
	channel <-chan *Result
}

// NewStandardInputPipe sets up a StandardInputPipe.
func NewStandardInputPipe() *StandardInputPipe {
	channel := make(chan *Result)
	logger.Go(func() {
		defer close(channel)

		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			bytes := append([]byte(nil), scanner.Bytes()...) // scanner reuses its buffer
			err := scanner.Err()
			channel <- &Result{Data: bytes, Error: err}
			if err != nil {
				return
			}
		}
	})
	return &StandardInputPipe{channel: channel}
}

// ReadChannel provides results which are single, uncompressed, decoded events.
func (c *StandardInputPipe) ReadChannel() <-chan *Result {
	return c.channel
}

// Close does nothing, as standard input closes automatically on EOF.
func (c *StandardInputPipe) Close() {}

// KinesisPipe is a ResultPipe that consumes globs of events from Kinesis.
type KinesisPipe struct {
	// C is used to read records off the kinsumer queue
	C <-chan *Result

	// send is a write only alias to C
	send chan<- *Result

	closer   chan struct{}
	kinsumer *kinsumer.Kinsumer
	sync.WaitGroup
}

func configEntryToDuration(entry string) (time.Duration, error) {
	if len(entry) == 0 {
		return 0, nil
	}
	d, e := time.ParseDuration(entry)
	if d < 0 {
		return 0, fmt.Errorf("%s is a negative duration", entry)
	}
	return d, e
}

func configToKinsumerConfig(config Config) (kinsumer.Config, error) {
	kinsumerConfig := kinsumer.NewConfig()
	d, e := configEntryToDuration(config.ThrottleDelay)
	if e != nil {
		return kinsumerConfig, fmt.Errorf("Invalid ThrottleDelay: %s", e)
	}
	if d > 0 {
		kinsumerConfig = kinsumerConfig.WithThrottleDelay(d)
	}

	d, e = configEntryToDuration(config.CommitFrequency)
	if e != nil {
		return kinsumerConfig, fmt.Errorf("Invalid CommitFrequency: %s", e)
	}
	if d > 0 {
		kinsumerConfig = kinsumerConfig.WithCommitFrequency(d)
	}

	d, e = configEntryToDuration(config.ShardCheckFrequency)
	if e != nil {
		return kinsumerConfig, fmt.Errorf("Invalid ShardCheckFrequency: %s", e)
	}
	if d > 0 {
		kinsumerConfig = kinsumerConfig.WithShardCheckFrequency(d)
	}

	if config.BufferSize < 0 {
		return kinsumerConfig, fmt.Errorf("Invalid (negative) BufferSize: %d", config.BufferSize)
	}
	if config.BufferSize > 0 {
		kinsumerConfig = kinsumerConfig.WithBufferSize(config.BufferSize)
	}

	return kinsumerConfig, nil
}

// NewKinesisPipe returns a newly created KinesisPipe.
func NewKinesisPipe(session *session.Session, stats statsd.Statter, config Config) (*KinesisPipe, error) {
	kinsumerConfig, err := configToKinsumerConfig(config)
	if err != nil {
		return nil, err
	}

	kinsumerConfig = kinsumerConfig.WithStats(kstatsd.NewWithStatter(stats))
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	kinsumer, err := kinsumer.NewWithSession(
		session,
		config.StreamName,
		config.ApplicationName,
		hostname,
		kinsumerConfig,
	)
	if err != nil {
		return nil, err
	}

	err = kinsumer.Run()
	if err != nil {
		return nil, err
	}

	channel := make(chan *Result)
	c := &KinesisPipe{
		kinsumer: kinsumer,
		send:     channel,
		C:        channel,
		closer:   make(chan struct{}),
	}
	c.Add(1)
	logger.Go(func() {
		defer c.Done()
		c.crank()
	})
	return c, nil
}

func (c *KinesisPipe) crank() {
	for {
		d, err := c.kinsumer.Next()
		select {
		case <-c.closer:
			return
		case c.send <- &Result{Data: d, Error: err}:
		}
	}
}

// ReadChannel provides Results which are base-64 encoded, compressed lists of JSON records.
func (c *KinesisPipe) ReadChannel() <-chan *Result {
	return c.C
}

// Close closes down Kinesis consumption.
func (c *KinesisPipe) Close() {
	if c.kinsumer != nil {
		c.kinsumer.Stop()
		close(c.closer)
		c.Wait()
	}
}
