package connector

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"go.nanomsg.org/mangos/v3"
	"go.uber.org/zap"
)

type HttpCollector struct {
	config    *config.CollectorConfig
	urlGroups []config.UrlGroupConfig
}

func NewHttpConnector(c *config.CollectorConfig, ugc []config.UrlGroupConfig) (*HttpCollector, error) {
	return &HttpCollector{config: c, urlGroups: ugc}, nil
}

func (r *HttpCollector) Collect(publisher mangos.Socket) {
	stream := make(chan []byte, 1)
	defer close(stream)
	go func() {
		for {
			if err := r.receive(stream); err != nil {
				logger.GetLogger().Warn(
					"Error while receiving data for the stream",
					zap.String("URL", r.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
	process(stream, r.config.Name, r.config.Protocol, publisher)
}

func (h *HttpCollector) receive(stream chan<- []byte) error {

	errors := make(chan error)
	done := make(chan bool)
	var wg sync.WaitGroup
	wg.Add(len(h.urlGroups))
	for _, url := range h.urlGroups {
		go func(url config.UrlGroupConfig) {
			poll(url, stream)
		}(url)
	}
	go func() {
		// if the reading of all register groups is finished close the done channel
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// all reading is done, break the select statement
		break
	case err := <-errors:
		close(errors)
		return err
	}
	return nil
}

func poll(ugc config.UrlGroupConfig, stream chan<- []byte) error {
	ticker := time.NewTicker(ugc.PollingInterval)
	done := make(chan struct{})
	for {
		select {
		case <-ticker.C:
			resp, err := http.Get(ugc.Url)
			// TODO: how to handle failed reads, never attempt again or keep trying
			if err != nil {
				return err
			}
			bytes, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			stream <- bytes
			resp.Body.Close()
		case <-done:
			ticker.Stop()
			return nil
		}
	}
}
