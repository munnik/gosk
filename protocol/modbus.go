package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/munnik/gosk/logger"
	"github.com/munnik/modbus"
	"go.uber.org/zap"
)

const (
	MODBUS_HEADER_LENGTH = 7

	// MODBUS_Maximum_Number_Of_Registers is maximum number of registers that can be read in one request, a modbus message is limit to 256 bytes
	// TODO: this should be checked when register groups are created
	MODBUS_MAXIMUM_NUMBER_OF_REGISTERS = 125
	MODBUS_MAXIMUM_NUMBER_OF_COILS     = 2000
)

const (
	// 01 (0x01) Read Coils
	ReadCoils = 0x01
	// 02 (0x02) Read Discrete Inputs
	ReadDiscreteInputs = 0x02
	// 03 (0x03) Read Holding Registers
	ReadHoldingRegisters = 0x03
	// 04 (0x04) Read Input Registers
	ReadInputRegisters = 0x04
	// 05 (0x05) Write Single Coil
	WriteSingleCoil = 0x05
	// 06 (0x06) Write Single Register
	WriteSingleRegister = 0x06
	// 08 (0x08) Diagnostics (Serial Line only)
	Diagnostics = 0x08
	// 11 (0x0B) Get Comm Event Counter (Serial Line only)
	GetCommEventCounter = 0x0B
	// 15 (0x0F) Write Multiple Coils
	WriteMultipleCoils = 0x0F
	// 16 (0x10) Write Multiple Registers
	WriteMultipleRegisters = 0x10
	// 17 (0x11) Report Server ID (Serial Line only)
	ReportServerID = 0x11
	// 22 (0x16) Mask Write Register
	MaskWriteRegisters = 0x16
	// 23 (0x17) Read/Write Multiple Registers
	ReadWriteMultipleRegisters = 0x17
	// 43 / 14 (0x2B / 0x0E) Read Device Identification
	ReadDeviceIdentificationA = 0x0E
	ReadDeviceIdentificationB = 0x43
)

type ModbusHeader struct {
	Slave                    uint8  `mapstructure:"slave"`
	FunctionCode             uint16 `mapstructure:"functionCode"`
	Address                  uint16 `mapstructure:"address"`
	NumberOfCoilsOrRegisters uint16 `mapstructure:"numberOfCoilsOrRegisters"`
}

// ModbusConnection is the transport that every register group of one
// modbus connector shares: a single socket or serial line, the lock that
// serialises requests over it, and the state needed to bring it back
// after it dies.
//
// Bringing it back is the point. modbus.Client never redials on its own:
// once its transport is up, Open returns ErrTransportIsAlreadyOpen and
// keeps handing out the same transport, even after the peer has closed
// it. Nothing else closed it either - only the Write path did, and only
// for the writes a subscriber sends - so a slave that dropped the
// connection wedged the connector for the lifetime of the process: every
// poll from then on wrote to the dead socket, logged "write: broken
// pipe" and tried the same dead socket again a polling interval later.
// On node-marinesolarenergy-test's ComAp genset controller, which resets
// the link after a stretch of unanswered requests, that ran for 18 hours
// straight and only ever ended because a deploy restarted the unit. Do
// closes the transport whenever an error means the connection itself is
// gone, so the next request dials a fresh one.
type ModbusConnection struct {
	realClient *modbus.Client

	// everything below is guarded by lock
	lock      sync.Mutex
	connected bool
	// failedDials backs the reconnect delay off while the slave is
	// simply not there, so a connector polling an absent device does
	// not dial it several times a second.
	failedDials int
	nextDial    time.Time
	// consecutiveTimeouts counts timed out requests since the last one
	// that got an answer, across all register groups - they share this
	// one transport, so a slave that has stopped answering shows up as
	// timeouts spread over all of them rather than as a run on any one.
	consecutiveTimeouts int
}

const (
	// MODBUS_MINIMUM_RECONNECT_DELAY is how long to wait before
	// redialling after the connection dropped, and the first step of
	// the backoff that follows failed dials.
	MODBUS_MINIMUM_RECONNECT_DELAY = 1 * time.Second
	// MODBUS_MAXIMUM_RECONNECT_DELAY caps that backoff, so a slave that
	// comes back after a long absence is picked up within this long.
	MODBUS_MAXIMUM_RECONNECT_DELAY = 30 * time.Second
	// MODBUS_TIMEOUTS_BEFORE_RECONNECT is how many timed out requests
	// in a row it takes before the connection is treated as gone, see
	// Do.
	MODBUS_TIMEOUTS_BEFORE_RECONNECT = 3
)

// ErrWaitingToReconnect is returned instead of attempting a request while
// the connection is down and the reconnect delay has not elapsed yet.
var ErrWaitingToReconnect = errors.New("waiting before reconnecting to the modbus slave")

func NewModbusConnection(realClient *modbus.Client) *ModbusConnection {
	return &ModbusConnection{realClient: realClient}
}

// Do runs f against the shared client with the connection open and the
// lock held, and decides from the error whether the connection survived.
func (c *ModbusConnection) Do(f func(*modbus.Client) error) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if err := c.open(); err != nil {
		return err
	}

	err := f(c.realClient)
	switch {
	case err == nil:
		c.consecutiveTimeouts = 0
	case errors.Is(err, modbus.ErrRequestTimedOut):
		// One timed out request is not proof the connection is gone -
		// the slave can just be busy - and redialling on every one of
		// them would churn the socket for nothing. Several in a row is
		// a different story: that is what a slave that has stopped
		// answering looks like from here, and on the ComAp it is
		// exactly the state that precedes the controller resetting the
		// connection outright.
		c.consecutiveTimeouts++
		if c.consecutiveTimeouts >= MODBUS_TIMEOUTS_BEFORE_RECONNECT {
			c.disconnect()
		}
	case IsTransportError(err):
		c.disconnect()
	}
	return err
}

// open makes sure there is a usable transport, without ever dialling more
// often than the backoff allows.
func (c *ModbusConnection) open() error {
	if c.connected {
		return nil
	}
	if time.Now().Before(c.nextDial) {
		// Fail fast rather than dial. modbus.Client dials with a 5
		// second timeout, and with every register group queueing on
		// this same lock, a slave that is not there would otherwise
		// block the whole connector for 5 seconds per group per round.
		return ErrWaitingToReconnect
	}
	if err := c.realClient.Open(); err != nil && !errors.Is(err, modbus.ErrTransportIsAlreadyOpen) {
		c.failedDials++
		c.nextDial = time.Now().Add(reconnectDelay(c.failedDials))
		return err
	}
	c.connected = true
	c.failedDials = 0
	c.consecutiveTimeouts = 0
	return nil
}

// disconnect drops the transport so the next request dials a fresh one.
func (c *ModbusConnection) disconnect() {
	if err := c.realClient.Close(); err != nil && !errors.Is(err, modbus.ErrTransportIsAlreadyClosed) {
		logger.GetLogger().Warn(
			"Could not close the modbus transport",
			zap.Error(err),
		)
	}
	c.connected = false
	c.consecutiveTimeouts = 0
	c.nextDial = time.Now().Add(MODBUS_MINIMUM_RECONNECT_DELAY)
}

func reconnectDelay(failedDials int) time.Duration {
	if failedDials < 1 {
		return MODBUS_MINIMUM_RECONNECT_DELAY
	}
	delay := MODBUS_MINIMUM_RECONNECT_DELAY << min(failedDials-1, 16)
	if delay <= 0 || delay > MODBUS_MAXIMUM_RECONNECT_DELAY {
		return MODBUS_MAXIMUM_RECONNECT_DELAY
	}
	return delay
}

// IsTransportError reports whether err means the connection itself is no
// longer usable, as opposed to the slave answering - with an exception
// response - a request it cannot serve.
//
// The distinction is what keeps one bad register group from taking the
// others down: a group asking for an address the slave does not have, or
// one the slave reports a device failure for, gets that same exception
// every polling interval forever, and dropping a perfectly healthy socket
// over it would interrupt every other group sharing it. A framing or unit
// id error, on the other hand, means this end and the slave no longer
// agree on where in the stream they are, which only a fresh connection
// fixes.
func IsTransportError(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, modbus.ErrIllegalFunction),
		errors.Is(err, modbus.ErrIllegalDataAddress),
		errors.Is(err, modbus.ErrIllegalDataValue),
		errors.Is(err, modbus.ErrServerDeviceFailure),
		errors.Is(err, modbus.ErrAcknowledge),
		errors.Is(err, modbus.ErrServerDeviceBusy),
		errors.Is(err, modbus.ErrMemoryParityError),
		errors.Is(err, modbus.ErrGWPathUnavailable),
		errors.Is(err, modbus.ErrGWTargetFailedToRespond),
		errors.Is(err, modbus.ErrUnexpectedParameters),
		errors.Is(err, ErrWaitingToReconnect):
		return false
	}
	return true
}

type ModbusClient struct {
	connection  *ModbusConnection
	header      *ModbusHeader
	writeHeader *ModbusHeader
	writeValues *[]byte
}

func NewModbusClient(connection *ModbusConnection, header *ModbusHeader, writeHeader *ModbusHeader, writeValues *[]byte) *ModbusClient {
	return &ModbusClient{
		connection:  connection,
		header:      header,
		writeHeader: writeHeader,
		writeValues: writeValues,
	}
}

func (m *ModbusClient) Read(bytes []byte) (int, error) {
	return m.execute(m.header, bytes)
}

func (m *ModbusClient) Write(bytes []byte) (int, error) {
	header, bytes, err := ExtractModbusHeader(bytes)
	if err != nil {
		return 0, err
	}
	return m.execute(header, bytes)
}

func (m *ModbusClient) execute(header *ModbusHeader, bytes []byte) (int, error) {
	var count int
	err := m.connection.Do(func(client *modbus.Client) error {
		var err error
		count, err = m.request(client, header, bytes)
		return err
	})
	return count, err
}

// request performs one modbus request on an already open client. The
// caller holds the connection, see Do.
//
// For a read the response, modbus header included, is written into bytes,
// which has to have the capacity for it, and the number of bytes written
// is returned. For a write bytes holds the values to write and its length
// is returned.
func (m *ModbusClient) request(client *modbus.Client, header *ModbusHeader, bytes []byte) (int, error) {
	var result []byte

	switch header.FunctionCode {
	case ReadCoils:
		values, err := client.ReadCoils(header.Address, header.NumberOfCoilsOrRegisters, modbus.WithUnitID(header.Slave))
		if err != nil {
			return 0, fmt.Errorf("error while reading slave %v coils %v, with length %v and function code %v, the error that occurred was %w", header.Slave, header.Address, header.NumberOfCoilsOrRegisters, header.FunctionCode, err)
		}
		result = CoilsToBytes(values)
	case ReadDiscreteInputs:
		values, err := client.ReadDiscreteInputs(header.Address, header.NumberOfCoilsOrRegisters, modbus.WithUnitID(header.Slave))
		if err != nil {
			return 0, fmt.Errorf("error while reading slave %v discrete inputs %v, with length %v and function code %v, the error that occurred was %w", header.Slave, header.Address, header.NumberOfCoilsOrRegisters, header.FunctionCode, err)
		}
		result = CoilsToBytes(values)
	case ReadHoldingRegisters:
		values, err := client.ReadRegisters(header.Address, header.NumberOfCoilsOrRegisters, modbus.HoldingRegister, modbus.WithUnitID(header.Slave))
		if err != nil {
			return 0, fmt.Errorf("error while reading slave %v holding register %v, with length %v and function code %v, the error that occurred was %w", header.Slave, header.Address, header.NumberOfCoilsOrRegisters, header.FunctionCode, err)
		}
		result = RegistersToBytes(values)
	case ReadInputRegisters:
		values, err := client.ReadRegisters(header.Address, header.NumberOfCoilsOrRegisters, modbus.InputRegister, modbus.WithUnitID(header.Slave))
		if err != nil {
			return 0, fmt.Errorf("error while reading slave %v input register %v, with length %v and function code %v, the error that occurred was %w", header.Slave, header.Address, header.NumberOfCoilsOrRegisters, header.FunctionCode, err)
		}
		result = RegistersToBytes(values)
	case WriteSingleCoil:
		if header.NumberOfCoilsOrRegisters != 1 {
			return 0, fmt.Errorf("expected only 1 register but got %d", header.NumberOfCoilsOrRegisters)
		}
		coils, err := BytesToCoils(bytes)
		if err != nil {
			return 0, err
		}
		if err := client.WriteCoil(header.Address, coils[0], modbus.WithUnitID(header.Slave)); err != nil {
			return 0, fmt.Errorf("error while writing slave %v coil %v, with function code %v, the error that occurred was %w", header.Slave, header.Address, header.FunctionCode, err)
		}
		return len(bytes), nil
	case WriteSingleRegister:
		if header.NumberOfCoilsOrRegisters != 1 {
			return 0, fmt.Errorf("expected only 1 register but got %d", header.NumberOfCoilsOrRegisters)
		}
		registers, err := BytesToRegisters(bytes)
		if err != nil {
			return 0, err
		}
		if len(registers) != int(header.NumberOfCoilsOrRegisters) {
			return 0, fmt.Errorf("expected %d registers but got %d register", header.NumberOfCoilsOrRegisters, len(registers))
		}
		if err := client.WriteRegister(header.Address, registers[0], modbus.WithUnitID(header.Slave)); err != nil {
			return 0, fmt.Errorf("error while writing slave %v register %v, with function code %v, the error that occurred was %w", header.Slave, header.Address, header.FunctionCode, err)
		}
		return len(bytes), nil
	case WriteMultipleCoils:
		coils, err := BytesToCoils(bytes)
		if err != nil {
			return 0, err
		}
		if err := client.WriteCoils(header.Address, coils, modbus.WithUnitID(header.Slave)); err != nil {
			return 0, fmt.Errorf("error while writing slave %v coils %v, with length %v and function code %v, the error that occurred was %w", header.Slave, header.Address, header.NumberOfCoilsOrRegisters, header.FunctionCode, err)
		}
		return len(bytes), nil
	case WriteMultipleRegisters:
		registers, err := BytesToRegisters(bytes)
		if err != nil {
			return 0, err
		}
		if len(registers) != int(header.NumberOfCoilsOrRegisters) {
			return 0, fmt.Errorf("expected %d registers but got %d register", header.NumberOfCoilsOrRegisters, len(registers))
		}
		if err := client.WriteRegisters(header.Address, registers, modbus.WithUnitID(header.Slave)); err != nil {
			return 0, fmt.Errorf("error while writing slave %v registers %v, with length %v and function code %v, the error that occurred was %w", header.Slave, header.Address, header.NumberOfCoilsOrRegisters, header.FunctionCode, err)
		}
		return len(bytes), nil
	default:
		return 0, fmt.Errorf("unsupported function code type %v", header.FunctionCode)
	}

	framed := InjectModbusHeader(header, result)
	if cap(bytes) < len(framed) {
		// Used to truncate silently: request wrote into bytes with
		// append, so a response larger than the buffer the caller sized
		// from its own register count quietly landed in a reallocated
		// array the caller never saw, and the caller then published
		// len(framed) bytes of its own stale buffer.
		return 0, fmt.Errorf("a buffer of %d bytes is too small for the %d byte response of slave %v address %v", cap(bytes), len(framed), header.Slave, header.Address)
	}
	return copy(bytes[:len(framed)], framed), nil
}

func (m *ModbusClient) Poll(stream chan<- []byte, pollingInterval time.Duration, writeDelay time.Duration) error {
	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()
	done := make(chan struct{})

	// lastError is the error the previous poll produced, empty when it
	// succeeded. A read that keeps failing fails identically every
	// polling interval, and a dropped connection used to put thousands
	// of copies of the same broken pipe warning in the journal - 13k of
	// them in one 18 hour stretch on node-marinesolarenergy-test - so
	// only the transitions are worth a warning, the repeats go to debug.
	var lastError string

	for {
		select {
		case <-ticker.C:
			// A fresh buffer per poll. The previous one is still on its
			// way through stream and gets published as is, so writing
			// the next response into it would rewrite a message that
			// has not been sent yet.
			bytes := make([]byte, 0, int(m.header.NumberOfCoilsOrRegisters)*2+MODBUS_HEADER_LENGTH)

			var n int
			err := m.connection.Do(func(client *modbus.Client) error {
				// The write and the read it belongs to run under one
				// Do, so no other register group can put a request
				// between them and change what is read back.
				if m.writeHeader != nil {
					if _, err := m.request(client, m.writeHeader, *m.writeValues); err != nil {
						return fmt.Errorf("error while writing before read, %w", err)
					}
					time.Sleep(writeDelay)
				}
				var err error
				n, err = m.request(client, m.header, bytes)
				return err
			})
			if err != nil {
				if err.Error() == lastError {
					logger.GetLogger().Debug(
						"Error while reading",
						zap.Error(err),
					)
				} else {
					logger.GetLogger().Warn(
						"Error while reading",
						zap.Error(err),
					)
					lastError = err.Error()
				}
				continue
			}
			if lastError != "" {
				logger.GetLogger().Info(
					"Reading recovered",
					zap.Uint8("slave", m.header.Slave),
					zap.Uint16("address", m.header.Address),
					zap.String("previous error", lastError),
				)
				lastError = ""
			}

			stream <- bytes[:n]
		case <-done:
			return nil
		}
	}
}

func CoilsToBytes(values []bool) []byte {
	bytes := make([]byte, len(values)*2)
	for i, v := range values {
		if v {
			bytes[i*2] = 0xff
			bytes[i*2+1] = 0x00
		}
	}
	return bytes
}

func BytesToCoils(bytes []byte) ([]bool, error) {
	registers, err := BytesToRegisters(bytes)
	if err != nil {
		return nil, err
	}
	return RegistersToCoils(registers), nil
}

func RegistersToBytes(values []uint16) []byte {
	bytes := make([]byte, 0, 2*len(values))
	out := make([]byte, 2)
	for _, v := range values {
		binary.BigEndian.PutUint16(out, v)
		bytes = append(bytes, out...)
	}
	return bytes
}

func BytesToRegisters(bytes []byte) ([]uint16, error) {
	if len(bytes)%2 != 0 {
		return nil, fmt.Errorf("expected even number of bytes, got %d bytes", len(bytes))
	}
	numberOfRegisters := len(bytes) / 2
	registers := make([]uint16, 0, numberOfRegisters)
	for i := 0; i < numberOfRegisters; i++ {
		registers = append(registers, binary.BigEndian.Uint16(bytes[i*2:i*2+2]))
	}

	return registers, nil
}

func RegistersToCoils(registers []uint16) []bool {
	coils := make([]bool, len(registers))
	for i := range registers {
		if registers[i] == 0xff00 {
			coils[i] = true
		}
	}
	return coils
}

func CoilsToRegisters(coils []bool) []uint16 {
	registers := make([]uint16, len(coils))
	for i := range coils {
		if coils[i] {
			registers[i] = 0xff00
		}
	}
	return registers
}

func InjectModbusHeader(header *ModbusHeader, bytes []byte) []byte {
	headerBytes := make([]byte, 0, MODBUS_HEADER_LENGTH)
	headerBytes = append(headerBytes, byte(header.Slave))
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, header.FunctionCode)
	headerBytes = append(headerBytes, out...)
	binary.BigEndian.PutUint16(out, header.Address)
	headerBytes = append(headerBytes, out...)
	binary.BigEndian.PutUint16(out, header.NumberOfCoilsOrRegisters)
	headerBytes = append(headerBytes, out...)

	return append(headerBytes, bytes...)
}

func ExtractModbusHeader(bytes []byte) (*ModbusHeader, []byte, error) {
	if len(bytes) < MODBUS_HEADER_LENGTH {
		return nil, nil, fmt.Errorf("unable to extract the modbus header, expected at least %d bytes but got %d", MODBUS_HEADER_LENGTH, len(bytes))
	}

	header := &ModbusHeader{
		Slave:                    uint8(bytes[0]),
		FunctionCode:             binary.BigEndian.Uint16(bytes[1:3]),
		Address:                  binary.BigEndian.Uint16(bytes[3:5]),
		NumberOfCoilsOrRegisters: binary.BigEndian.Uint16(bytes[5:7]),
	}

	return header, bytes[MODBUS_HEADER_LENGTH:], nil
}
