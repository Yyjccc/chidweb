package server

import (
	"chidweb/pkg/common"
	"fmt"
	"io"
	"sync"
	"time"
)

type CircularBuffer struct {
	data  []byte
	size  int
	read  int
	write int
}

func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		data: make([]byte, size),
		size: size,
	}
}

type TunnelStatus int

const (
	TunnelStateNew TunnelStatus = iota
	TunnelStateConnected
	TunnelStateDisconnected
)

type TunnelState struct {
	conn      *Connection
	buffer    *CircularBuffer
	status    TunnelStatus
	lastRetry time.Time
}

type TunnelCoordinator struct {
	bufferPool  *sync.Pool
	tunnelState sync.Map
	done        chan struct{}
}

func NewTunnelCoordinator() *TunnelCoordinator {
	tm := &TunnelCoordinator{
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, 4096)
			},
		},
		done: make(chan struct{}),
	}
	return tm
}

func (tm *TunnelCoordinator) CreateTunnel(conn *Connection) {
	state := &TunnelState{
		conn:      conn,
		buffer:    NewCircularBuffer(MaxBufferSize),
		status:    TunnelStateNew,
		lastRetry: time.Now(),
	}
	tm.tunnelState.Store(conn.ID(), state)
	go tm.manageTunnel(state)
}

func (tm *TunnelCoordinator) manageTunnel(state *TunnelState) {
	for {
		select {
		case <-tm.done:
			return
		case <-state.conn.Done:
			return
		default:
			tm.handleTunnelState(state)
			time.Sleep(time.Second)
		}
	}
}

func (tm *TunnelCoordinator) handleTunnelState(state *TunnelState) {
	switch state.status {
	case TunnelStateDisconnected:
		if time.Since(state.lastRetry) > ReconnectInterval {
			tm.tryReconnect(state)
		}
	case TunnelStateConnected:
		tm.handleData(state)
	}
}

const (
	MaxBufferSize     = 1024 * 1024 // 1MB buffer size
	ReconnectInterval = 5 * time.Second
)

func (cb *CircularBuffer) Write(data []byte) (int, error) {
	if len(data) > cb.size {
		return 0, fmt.Errorf("data too large for buffer")
	}

	n := copy(cb.data[cb.write:], data)
	cb.write = (cb.write + n) % cb.size
	return n, nil
}

func (cb *CircularBuffer) Read(p []byte) (int, error) {
	if cb.read == cb.write {
		return 0, io.EOF
	}

	n := copy(p, cb.data[cb.read:cb.write])
	cb.read = (cb.read + n) % cb.size
	return n, nil
}

func (tm *TunnelCoordinator) handleData(state *TunnelState) {
	buffer := tm.bufferPool.Get().([]byte)
	defer tm.bufferPool.Put(buffer)

	n, err := state.buffer.Read(buffer)
	if err != nil && err != io.EOF {
		state.status = TunnelStateDisconnected
		return
	}

	if n > 0 {
		packet := &common.Packet{
			Type:     common.PacketTypeData,
			ClientID: state.conn.ClientID,
			ConnID:   state.conn.ConnID,
			Payload:  buffer[:n],
		}
		state.conn.PendingPackets <- packet
	}
}

func (tm *TunnelCoordinator) tryReconnect(state *TunnelState) {
	state.lastRetry = time.Now()
	packet := &common.Packet{
		Type:     common.PacketTypeConnect,
		ClientID: state.conn.ClientID,
		ConnID:   state.conn.ConnID,
	}
	state.conn.PendingPackets <- packet
	state.status = TunnelStateConnected
}

func (c *Connection) ID() string {
	return fmt.Sprintf("%d-%d", c.ClientID, c.ConnID)
}
