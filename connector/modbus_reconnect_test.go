package connector

import (
	"encoding/binary"
	"io"
	"net"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/munnik/gosk/config"
	"github.com/munnik/gosk/protocol"
)

// fakeModbusSlave serves read input register requests over modbus/tcp and
// hangs up on the spot whenever dropConnection is set, the way the ComAp
// genset controller on node-marinesolarenergy-test resets the link after
// a stretch of unanswered requests. It counts the connections it accepts
// so a test can tell a reconnect from a connection that was never
// dropped.
func fakeModbusSlave(t *testing.T, dropConnection *atomic.Bool, accepted *atomic.Int32) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			go func() {
				defer conn.Close()
				header := make([]byte, 7)
				for {
					if _, err := io.ReadFull(conn, header); err != nil {
						return
					}
					pdu := make([]byte, int(binary.BigEndian.Uint16(header[4:6]))-1)
					if _, err := io.ReadFull(conn, pdu); err != nil {
						return
					}
					if dropConnection.Load() {
						return
					}
					quantity := binary.BigEndian.Uint16(pdu[3:5])
					response := append([]byte{}, header[0:4]...)
					response = binary.BigEndian.AppendUint16(response, 3+2*quantity)
					response = append(response, header[6], pdu[0], byte(2*quantity))
					response = append(response, make([]byte, 2*quantity)...)
					if _, err := conn.Write(response); err != nil {
						return
					}
				}
			}()
		}
	}()
	return listener
}

// TestModbusConnectorReconnectsAfterTheSlaveDropsTheConnection guards the
// bug that left gosk-connectComAp on node-marinesolarenergy-test polling a
// dead socket for 18 hours at a stretch: modbus.Client never redials by
// itself, and nothing in the read path ever closed the transport, so once
// the ComAp reset the connection every later poll wrote to the same dead
// socket and logged "write: broken pipe" until a deploy restarted the
// unit. See protocol.ModbusConnection.
func TestModbusConnectorReconnectsAfterTheSlaveDropsTheConnection(t *testing.T) {
	var dropConnection atomic.Bool
	var accepted atomic.Int32
	listener := fakeModbusSlave(t, &dropConnection, &accepted)
	defer listener.Close()

	u, err := url.Parse("tcp://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	c := &config.ConnectorConfig{
		Name: "test", URL: u, Protocol: config.ModbusType,
		DataBits: 8, StopBits: "1", Parity: "N", Timeout: time.Second,
	}
	var rgcs []config.RegisterGroupConfig
	for _, address := range []uint16{1043, 1089, 1092} {
		rgcs = append(rgcs, config.RegisterGroupConfig{
			ModbusHeader: protocol.ModbusHeader{
				Slave: 1, FunctionCode: protocol.ReadInputRegisters,
				Address: address, NumberOfCoilsOrRegisters: 1,
			},
			PollingInterval: 200 * time.Millisecond,
		})
	}
	conn, err := NewModbusConnector(c, rgcs)
	if err != nil {
		t.Fatalf("NewModbusConnector: %v", err)
	}

	stream := make(chan []byte, 100)
	go conn.receive(stream)

	awaitData := func(what string) {
		t.Helper()
		select {
		case <-stream:
		case <-time.After(10 * time.Second):
			t.Fatalf("no data %s", what)
		}
	}

	awaitData("before the connection was dropped")

	dropConnection.Store(true)
	time.Sleep(time.Second)
	dropConnection.Store(false)

	// drain whatever was already in flight when the slave hung up, so
	// what follows can only be data read over a new connection
	for len(stream) > 0 {
		<-stream
	}
	awaitData("after the slave dropped the connection")

	if got := accepted.Load(); got < 2 {
		t.Fatalf("the slave accepted %d connection(s), so the connector reused the dropped one instead of redialling", got)
	}
}

// TestModbusConnectionBacksOffWhileTheSlaveIsAbsent checks that a slave
// that is not there does not cost every register group a dial attempt per
// poll. modbus.Client dials with a 5 second timeout and all register
// groups queue on one lock, so dialling per poll would stall the whole
// connector for far longer than its polling interval.
func TestModbusConnectionBacksOffWhileTheSlaveIsAbsent(t *testing.T) {
	// a listener that is closed right away gives us a port nothing is
	// listening on, so dialling it fails immediately
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	address := listener.Addr().String()
	listener.Close()

	u, err := url.Parse("tcp://" + address)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	c := &config.ConnectorConfig{
		Name: "test", URL: u, Protocol: config.ModbusType,
		DataBits: 8, StopBits: "1", Parity: "N", Timeout: time.Second,
	}
	conn, err := NewModbusConnector(c, []config.RegisterGroupConfig{{
		ModbusHeader: protocol.ModbusHeader{
			Slave: 1, FunctionCode: protocol.ReadInputRegisters,
			Address: 1043, NumberOfCoilsOrRegisters: 1,
		},
		PollingInterval: 10 * time.Millisecond,
	}})
	if err != nil {
		t.Fatalf("NewModbusConnector: %v", err)
	}

	client := protocol.NewModbusClient(conn.connection, &protocol.ModbusHeader{
		Slave: 1, FunctionCode: protocol.ReadInputRegisters,
		Address: 1043, NumberOfCoilsOrRegisters: 1,
	}, nil, nil)

	bytes := make([]byte, 0, 2+protocol.MODBUS_HEADER_LENGTH)
	if _, err := client.Read(bytes); err == nil {
		t.Fatal("reading from a slave that is not there should fail")
	}
	// the dial failed, so the next attempt has to wait out the backoff
	// rather than dial again
	if _, err := client.Read(bytes); err == nil {
		t.Fatal("reading from a slave that is not there should fail")
	} else if err != protocol.ErrWaitingToReconnect {
		t.Fatalf("expected the second read to be held off by the reconnect backoff, got %v", err)
	}
}
