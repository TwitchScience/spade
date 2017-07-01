package eventmetadata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"time"

	"github.com/twitchscience/aws_utils/logger"
	"github.com/twitchscience/scoop_protocol/scoop_protocol"
	"github.com/twitchscience/spade/config_fetcher/fetcher"
	"github.com/twitchscience/spade/reporter"
	"github.com/twitchscience/spade/transformer"
)

// DynamicLoader fetches configs on an interval, with stats on the fetching process.
type DynamicLoader struct {
	fetcher    fetcher.ConfigFetcher
	reloadTime time.Duration
	retryDelay time.Duration
	// TEMP: change back to [] when /allmetadata endpoint is done
	// configs    []scoop_protocol.EventMetadataConfig
	configs scoop_protocol.EventMetadataConfig

	closer chan bool
	stats  reporter.StatsLogger
}

// NewDynamicLoader returns a new DynamicLoader, performing the first fetch.
func NewDynamicLoader(
	fetcher fetcher.ConfigFetcher,
	reloadTime,
	retryDelay time.Duration,
	stats reporter.StatsLogger,
) (*DynamicLoader, error) {
	logger.Info("[Fred] config_loader.go NewDynamicLoader begin")
	d := DynamicLoader{
		fetcher:    fetcher,
		reloadTime: reloadTime,
		retryDelay: retryDelay,

		// TEMP: change back to [] when /allmetadata endpoint is done
		// configs:    []scoop_protocol.EventMetadataConfig{},
		configs: scoop_protocol.EventMetadataConfig{},
		closer:  make(chan bool),
		stats:   stats,
	}
	logger.Info("[Fred] config_loader.go NewDynamicLoader after d := DynamicLoader")

	config, err := d.retryPull(5, retryDelay)
	if err != nil {
		return nil, err
	}
	d.configs = config

	logger.Info("[Fred]config_loader.go NewDyanmicLoader")
	logger.Info(config.Metadata["spade_testing_3"])
	return &d, nil
}

// GetMetadataValueByType returns the metadata value given an eventName and metadataType.
func (d *DynamicLoader) GetMetadataValueByType(eventName string, metadataType string) (string, error) {
	if metadataType != string(scoop_protocol.COMMENT) && metadataType != string(scoop_protocol.EDGE_TYPE) {
		return "", transformer.ErrInvalidMetadataType{
			What: fmt.Sprintf("%s is not being tracked", eventName),
		}
	}

	if eventMetadata, found := d.configs.Metadata[eventName]; found {
		if metadataRow, exists := eventMetadata[metadataType]; exists {
			return metadataRow.MetadataValue, nil
		}
	}

	// Update error later
	return "", errors.New("Not found")
	// if transformArray, exists := d.configs[eventName]; exists {
	// 	return transformArray, nil
	// }
	// return nil, transformer.ErrNotTracked{
	// 	What: fmt.Sprintf("%s is not being tracked", eventName),
	// }
}

// TEMP: change back to [] when /allmetadata endpoint is done
// func (d *DynamicLoader) retryPull(n int, waitTime time.Duration) ([]scoop_protocol.EventMetadataConfig, error) {
func (d *DynamicLoader) retryPull(n int, waitTime time.Duration) (scoop_protocol.EventMetadataConfig, error) {
	var err error
	// TEMP: change back to [] when /allmetadata endpoint is done
	// var config    []scoop_protocol.EventMetadataConfig
	var config scoop_protocol.EventMetadataConfig
	for i := 1; i <= n; i++ {
		config, err = d.pullConfigIn()
		if err == nil {
			return config, nil
		}
		time.Sleep(waitTime * time.Duration(i))
	}
	// TEMP: change back to nil, err when /allmetadata endpoint is done
	// return nil, err
	return config, err
}

// TEMP: change back to [] when /allmetadata endpoint is done
// func (d *DynamicLoader) pullConfigIn() ([]scoop_protocol.EventMetadataConfig, error) {
func (d *DynamicLoader) pullConfigIn() (scoop_protocol.EventMetadataConfig, error) {
	logger.Info("[Fred] config_loader.go pullConfigIn begin")
	configReader, err := d.fetcher.Fetch()
	if err != nil {
		// TEMP: Remove var config...when /allmetadata endpoint is done
		var config scoop_protocol.EventMetadataConfig
		// return nil, err
		return config, err
	}
	logger.Info("[Fred] config_loader.go pullConfigIn no Fetch() error")

	b, err := ioutil.ReadAll(configReader)
	logger.Info("[Fred] config_loader.go pullConfigIn Read bytes")
	// logger.Info(b)
	if err != nil {
		// TEMP: Remove var config...when /allmetadata endpoint is done
		var config scoop_protocol.EventMetadataConfig
		// return nil, err
		return config, err
	}
	logger.Info("[Fred] config_loader.go pullConfigIn no ReadAll() error")
	// TEMP: change back to [] when /allmetadata endpoint is done
	// var cfgs []scoop_protocol.EventMetadataConfig
	cfgs := scoop_protocol.EventMetadataConfig{
		Metadata: make(map[string](map[string]scoop_protocol.EventMetadataRow)),
	}
	err = json.Unmarshal(b, &cfgs.Metadata)
	if err != nil {
		// TEMP: change back to [] when /allmetadata endpoint is done
		// return []scoop_protocol.EventMetadataConfig{}, err
		return scoop_protocol.EventMetadataConfig{}, err
	}
	logger.Info(cfgs.Metadata)
	logger.Info(cfgs.Metadata["spade_testing_3"])
	return cfgs, nil
}

// Close stops the DynamicLoader's fetching process.
func (d *DynamicLoader) Close() {
	d.closer <- true
}

// Crank is a blocking function that refreshes the config on an interval.
func (d *DynamicLoader) Crank() {
	// Jitter reload
	tick := time.NewTicker(d.reloadTime + time.Duration(rand.Intn(100))*time.Millisecond)
	for {
		select {
		case <-tick.C:
			// can put a circuit breaker here.
			now := time.Now()
			newConfig, err := d.retryPull(5, d.retryDelay)
			if err != nil {
				logger.WithError(err).Error("Failed to refresh config")
				d.stats.Timing("config.error", time.Since(now))
				continue
			}
			d.stats.Timing("config.success", time.Since(now))
			d.configs = newConfig
		case <-d.closer:
			return
		}
	}
}
