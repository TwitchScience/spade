package writer

import (
	"sync"

	"github.com/twitchscience/aws_utils/logger"
)

// Multee implements the `SpadeWriter` and 'SpadeWriterManager' interface and forwards all calls
// to a map of targets.
type Multee struct {
	// targets is the spadewriters we will Multee events to
	targets map[string]SpadeWriter
	sync.RWMutex
}

// SpadeWriterManager allows operations on a set of SpadeWriters
type SpadeWriterManager interface {
	Add(key string, w SpadeWriter)
	Drop(key string)
	Replace(key string, newWriter SpadeWriter)
}

// Add adds a new writer to the target map
func (t *Multee) Add(key string, w SpadeWriter) {
	t.Lock()
	defer t.Unlock()
	_, exists := t.targets[key]
	if exists {
		logger.WithField("key", key).Error("Could not add SpadeWriter due to key collision")
		return
	}
	t.targets[key] = w
}

// NewMultee makes a empty multee and returns it
func NewMultee() *Multee {
	return &Multee{
		targets: make(map[string]SpadeWriter),
	}
}

// Drop drops an existing writer from the target map
func (t *Multee) Drop(key string) {
	t.Lock()
	defer t.Unlock()
	logger.WithField("key", key).Info("Dropping writer...")
	writer, exists := t.targets[key]
	if !exists {
		logger.WithField("key", key).Error("Could not drop SpadeWriter due to non existent key")
		return
	}
	logger.Go(func() {
		err := writer.Close()
		if err != nil {
			logger.WithError(err).
				WithField("writer_key", key).
				Error("Failed to close SpadeWriter on drop")
		}
	})
	delete(t.targets, key)
	logger.WithField("key", key).Info("Done dropping writer")
}

// Replace adds a new writer to the target map
func (t *Multee) Replace(key string, newWriter SpadeWriter) {
	t.Lock()
	defer t.Unlock()
	logger.WithField("key", key).Info("Replacing writer...")
	oldWriter, exists := t.targets[key]
	if !exists {
		logger.WithField("key", key).Error("Could not replace SpadeWriter due to non existent key")
		return
	}
	logger.Go(func() {
		err := oldWriter.Close()
		if err != nil {
			logger.WithError(err).
				WithField("writer_key", key).
				Error("Failed to close SpadeWriter on replace")
		}
	})
	delete(t.targets, key)
	t.targets[key] = newWriter
	logger.WithField("key", key).Info("Done replacing writer")
}

// Write forwards a writerequest to multiple targets
func (t *Multee) Write(r *WriteRequest) {
	t.RLock()
	defer t.RUnlock()

	for _, writer := range t.targets {
		writer.Write(r)
	}
}

// Rotate forwards a rotation request to multiple targets
func (t *Multee) Rotate() (bool, error) {
	t.RLock()
	defer t.RUnlock()

	allDone := true
	for k, writer := range t.targets {
		// losing errors here. Alternatives are to
		// not rotate writers further down the
		// chain, or to return an arbitrary error
		// out of all possible ones that occured
		done, err := writer.Rotate()
		if err != nil {
			logger.WithError(err).WithField("writer_key", k).Error("Failed to forward rotation request")
			allDone = false
		} else {
			allDone = allDone && done
		}
	}
	return allDone, nil
}

// Close closes all the target writers, it does this asynchronously
func (t *Multee) Close() error {
	t.Lock()
	defer t.Unlock()

	var wg sync.WaitGroup
	wg.Add(len(t.targets))
	for key, writer := range t.targets {
		k := key
		w := writer
		// losing errors here. Alternative is to
		// return an arbitrary error out of all
		// possible ones that occured
		logger.Go(func() {
			defer wg.Done()
			err := w.Close()
			if err != nil {
				logger.WithError(err).
					WithField("writer_key", k).
					Error("Failed to forward rotation request")
			}
		})
	}

	wg.Wait()
	return nil
}
