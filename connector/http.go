package connector

import (
	"fmt"
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

// NewHttpConnector validates the url groups it will poll. A group with no
// polling interval used to reach time.NewTicker(0) in poll, which panics -
// taking the whole connector down at startup over a missing line in a
// config file, when the point of this connector (see process) is to stay up
// and report even when the thing it polls is unreachable. Fail here, where
// the reason can be stated, instead.
func NewHttpConnector(c *config.ConnectorConfig, ugc []config.UrlGroupConfig) (*HttpConnector, error) {
	if len(ugc) == 0 {
		// Not fatal - see receive - but always a configuration gap rather
		// than something intentional, so it is worth saying at startup.
		logger.GetLogger().Warn(
			"HTTP connector configured with no url groups, it will never publish any data",
			zap.String("Name", c.Name),
		)
	}
	for _, url := range ugc {
		if url.Url == "" {
			return nil, fmt.Errorf("url group without a url: %v", url)
		}
		if url.PollingInterval <= 0 {
			return nil, fmt.Errorf("polling interval for %v must be positive, got %v", url.Url, url.PollingInterval)
		}
	}
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
	process(stream, r.config.Name, r.config.Protocol, publisher, r.config.Timeout)
}

func (*HttpConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	// do nothing
}

// receive polls every configured URL group until they have all stopped.
//
// The goroutines are joined properly now: poll's wg.Done was never
// deferred (and poll has no return path of its own today), so the
// WaitGroup could never reach zero and the done channel it gated was never
// closed - this select simply blocked forever on two channels that nothing
// could ever signal. The error channel it also selected on was never
// written to by anything at all, so it is gone.
func (h *HttpConnector) receive(stream chan<- []byte) error {
	// With no url groups configured - a misconfiguration, see
	// NewHttpConnector's warning - there is nothing for wg.Wait below to
	// wait on, so this would return immediately and Publish's
	// `for { h.receive(stream) }` would spin as fast as the scheduler
	// allows. Block forever instead: process already reports this
	// connector as DisconnectedOrNoData, which is accurate, at no cost.
	// ModbusConnector.receive does the same for the same reason.
	if len(h.urlGroups) == 0 {
		select {}
	}

	var wg sync.WaitGroup
	wg.Add(len(h.urlGroups))
	for _, url := range h.urlGroups {
		go func(url config.UrlGroupConfig) {
			defer wg.Done()
			h.poll(url, stream)
		}(url)
	}
	wg.Wait()
	return nil
}

func (h *HttpConnector) poll(ugc config.UrlGroupConfig, stream chan<- []byte) {
	ticker := time.NewTicker(ugc.PollingInterval)
	defer ticker.Stop()

	for range ticker.C {
		// TODO: how to handle failed reads, never attempt again or keep trying
		body, err := get(ugc.Url)
		if err != nil {
			logger.GetLogger().Error("Could not GET page", zap.String("URL", ugc.Url), zap.Error(err))
			continue
		}
		stream <- body
	}
}

// get fetches url and returns its body, always closing the response.
// Reading the body used to `continue` on error without closing it, leaking
// the connection on every failed read.
func get(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
