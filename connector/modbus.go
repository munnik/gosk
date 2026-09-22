package connector

import (
	"fmt"
	"sync"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/logger"
	"github.com/munnik/gosk/message"
	"github.com/munnik/gosk/nanomsg"
	"github.com/munnik/gosk/protocol"
	"github.com/munnik/modbus"
	"go.bug.st/serial"
	"go.uber.org/zap"
)

type ModbusConnector struct {
	config               *config.ConnectorConfig
	registerGroupsConfig []config.RegisterGroupConfig
	connection           *protocol.ModbusConnection
}

func NewModbusConnector(c *config.ConnectorConfig, rgcs []config.RegisterGroupConfig) (*ModbusConnector, error) {
	if len(rgcs) == 0 {
		// Not fatal - see receive's doc comment for why this connector
		// blocks instead of polling - but worth a visible warning at
		// startup, since a modbus connector with nothing to poll is
		// always a configuration gap, never intentional.
		logger.GetLogger().Warn(
			"Modbus connector configured with no register groups, it will never publish any data",
			zap.String("Name", c.Name),
		)
	}
	for _, rgc := range rgcs {
		// TODO add write function codes
		if rgc.FunctionCode == protocol.ReadCoils || rgc.FunctionCode == protocol.ReadDiscreteInputs {
			if rgc.NumberOfCoilsOrRegisters > protocol.MODBUS_MAXIMUM_NUMBER_OF_COILS {
				return nil, fmt.Errorf("maximum number %v of coils exceeded for register group %v", protocol.MODBUS_MAXIMUM_NUMBER_OF_COILS, rgc)
			}
		} else {
			if rgc.NumberOfCoilsOrRegisters > protocol.MODBUS_MAXIMUM_NUMBER_OF_REGISTERS {
				return nil, fmt.Errorf("maximum number %v of registers exceeded for register group %v", protocol.MODBUS_MAXIMUM_NUMBER_OF_REGISTERS, rgc)
			}
		}
		if !rgc.WriteBeforeRead.IsEmpty() {
			if rgc.Slave != rgc.WriteBeforeRead.Slave {
				return nil, fmt.Errorf("slave IDs for read and write should match, got %v and %v", rgc.Slave, rgc.WriteBeforeRead.Slave)
			}
			if rgc.WriteBeforeRead.FunctionCode < protocol.WriteSingleCoil {
				return nil, fmt.Errorf("function code should be a write, got %v", rgc.WriteBeforeRead.FunctionCode)
			}
			if rgc.WriteBeforeRead.Delay > rgc.PollingInterval {
				return nil, fmt.Errorf("write delay larger than polling interval, got %v and %v", rgc.WriteBeforeRead.Delay, rgc.PollingInterval)
			}
		}
		if rgc.FunctionCode >= protocol.WriteSingleCoil {
			return nil, fmt.Errorf("function code should be a read, got %v", rgc.FunctionCode)
		}
	}
	cc := &modbus.Configuration{
		URL:      c.URL.String(),
		Speed:    c.BaudRate,
		DataBits: c.DataBits,
	}
	switch c.StopBits {
	case "1":
		cc.StopBits = serial.OneStopBit
	case "1.5":
		cc.StopBits = serial.OnePointFiveStopBits
	case "2":
		cc.StopBits = serial.TwoStopBits
	default:
		return nil, fmt.Errorf("unsupport stop bits: %s", c.StopBits)
	}
	switch c.Parity {
	case "N":
		cc.Parity = serial.NoParity
	case "O":
		cc.Parity = serial.OddParity
	case "E":
		cc.Parity = serial.EvenParity
	default:
		return nil, fmt.Errorf("unsupport parity: %s", c.Parity)
	}
	realClient, err := modbus.NewClient(cc)
	if err != nil {
		return nil, fmt.Errorf("unable to create modbus client %v, the error that occurred was %v", c.URL.String(), err)
	}

	return &ModbusConnector{
		config:               c,
		registerGroupsConfig: rgcs,
		connection:           protocol.NewModbusConnection(realClient),
	}, nil
}

func (m *ModbusConnector) Publish(publisher *nanomsg.Publisher[message.Raw]) {
	stream := make(chan []byte, 1)
	defer close(stream)
	go func() {
		for {
			if err := m.receive(stream); err != nil {
				logger.GetLogger().Warn(
					"Error while receiving data for the stream",
					zap.String("URL", m.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
	process(stream, m.config.Name, m.config.Protocol, publisher, m.config.Timeout)
}

func (m *ModbusConnector) Subscribe(subscriber *nanomsg.Subscriber[message.Raw]) {
	go func() {
		client := protocol.NewModbusClient(
			m.connection,
			nil, // no need to set this because it will not be used in the Write([]byte) function
			nil,
			nil,
		)
		receiveBuffer := make(chan *message.Raw, bufferCapacity)
		go subscriber.Receive(receiveBuffer)

		for raw := range receiveBuffer {
			if _, err := client.Write(raw.Value); err != nil {
				logger.GetLogger().Warn(
					"Error while writing data",
					zap.String("URL", m.config.URL.String()),
					zap.String("Error", err.Error()),
				)
			}
		}
	}()
}

// receive polls every configured register group until they have all
// stopped, and reports the first error any of them produced. Publish's
// loop then restarts the whole set.
//
// Waiting for all of them, rather than returning on the first error, is
// what makes that restart safe. Returning early closed the error channel
// while the other groups were still polling, so the next one to fail
// panicked the process with a send on a closed channel, and the restart
// added a second full set of pollers on top of the ones still running -
// each generation contending harder for the shared modbus connection
// than the last. Neither is reachable today only because
// ModbusClient.Poll has no path that returns at all: it logs a failed
// read and tries again on the next tick, over a connection
// protocol.ModbusConnection redials for it when the failure means the
// connection itself is gone. So the error plumbing here has never
// actually run. That makes this latent rather than live - and it stays
// latent instead of becoming a crash the day Poll does return.
func (m *ModbusConnector) receive(stream chan<- []byte) error {
	// A connector with no register groups configured at all - a
	// misconfiguration (see NewModbusConnector's warning), not a
	// transient condition - has nothing for wg.Wait below to wait on,
	// so it would return immediately instead of blocking the way a
	// real register group's never-returning Poll normally does (see
	// this function's own doc comment). Publish's `for { m.receive
	// (stream) }` would then spin as fast as the scheduler allows -
	// observed on node-lambert-lambert burning more than a full CPU
	// core per idle connector, entirely from channel/waitgroup
	// allocation churn. Block forever instead: connector/main.go's
	// process already reports this connector as DisconnectedOrNoData -
	// which is accurate, it never publishes anything - at zero cost.
	if len(m.registerGroupsConfig) == 0 {
		select {}
	}

	// buffered by one per group so a failing poller can always report and
	// exit, whether or not anyone is still selecting on the channel
	errors := make(chan error, len(m.registerGroupsConfig))

	var wg sync.WaitGroup
	wg.Add(len(m.registerGroupsConfig))

	// start a go routine for each register group, if an error occurs send it on the error channel
	for i, rgc := range m.registerGroupsConfig {
		go func(i int, rgc config.RegisterGroupConfig) {
			defer wg.Done()

			// Spread the register groups evenly over the polling
			// interval instead of letting them all start at once. They
			// share one connection and one lock, so a round that all
			// starts at the same instant is a round that runs entirely
			// back to back - harmless while the slave answers in
			// milliseconds, but once it starts timing out, each group
			// waits out the full request timeout while holding the lock
			// and the whole connector collapses to one request per
			// timeout. That is what the ComAp failures on
			// node-marinesolarenergy-test look like in the journal just
			// before the controller resets the connection: seven
			// register groups taking turns, one error per second, none
			// of them anywhere near their 5 second interval.
			time.Sleep(time.Duration(i) * rgc.PollingInterval / time.Duration(len(m.registerGroupsConfig)))

			client := protocol.NewModbusClient(
				m.connection,
				rgc.ExtractModbusHeader(),
				rgc.ExtractWriteModbusHeader(), //TODO make sure this is nil when not configured
				&rgc.WriteBeforeRead.Values,
			)
			logger.GetLogger().Info("Created a new modbus cient",
				zap.Uint8("slave", rgc.Slave),
				zap.Uint16("function code", rgc.FunctionCode),
				zap.Uint16("address", rgc.Address),
				zap.Uint16("number of coils or registers", rgc.NumberOfCoilsOrRegisters),
			)
			if err := client.Poll(stream, rgc.PollingInterval, rgc.WriteBeforeRead.Delay); err != nil {
				logger.GetLogger().Error(
					"An error occurred while polling the client",
					zap.Error(err),
				)
				errors <- err
			}
		}(i, rgc)
	}

	wg.Wait()
	close(errors)

	// report the first failure, if any; the rest are already logged above
	for err := range errors {
		return err
	}
	return nil
}
