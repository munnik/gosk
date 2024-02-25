package connector

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"go.uber.org/zap"
)

type HttpConnector struct {
	config    *config.ConnectorConfig
	urlGroups []config.UrlGroupConfig
}

func NewHttpConnector(c *config.ConnectorConfig, ugc []config.UrlGroupConfig) (*HttpConnector, error) {
	return &HttpConnector{config: c, urlGroups: ugc}, nil
}

func (r *HttpConnector) Publish(publisher *nanomsg.Publisher[message.Raw]) {
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

func (*HttpConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	// do nothing
}

func (h *HttpConnector) receive(stream chan<- []byte) error {
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

func poll(ugc config.UrlGroupConfig, stream chan<- []byte) {
	ticker := time.NewTicker(ugc.PollingInterval)
	done := make(chan struct{})
Loop:
	for {
		select {
		case <-ticker.C:
			resp, err := http.Get(ugc.Url)
			// TODO: how to handle failed reads, never attempt again or keep trying
			if err != nil {
				logger.GetLogger().Error("Could not GET page", zap.Error(err))
				continue
			}
			bytes, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.GetLogger().Error("Could not read response body", zap.Error(err))
				continue
			}
			stream <- bytes
			resp.Body.Close()
		case <-done:
			ticker.Stop()
			break Loop
		}
	}
}
