package common

import (
	"bytes"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"sync"
	"time"
)

var (
	buffSize         = 4096
	packetChannelMax = 100
)

// tcp 连接
type Tunnel struct {
	Conn           net.Conn
	ClientId       uint32
	ID             uint32
	PendingPackets chan *Packet // 待发送的数据包队列
	Alive          bool
	Done           chan struct{}        // 连接关闭信号
	LastActive     time.Time            // 最后活跃时间
	OnClose        func(tunnel *Tunnel) // 回调函数，关闭隧道的时候执行
	SendBuffer     *bytes.Buffer        // 发送缓冲区
	BufferMutex    sync.Mutex
}

func NewTcpTunnel(conn net.Conn, onClose func(tunnel *Tunnel)) *Tunnel {
	uuid := Generate32ID()
	Info("[tunnel] open a tunnel , connect id: %d", uuid)
	tunnel := &Tunnel{
		Conn:           conn,
		Done:           make(chan struct{}),
		PendingPackets: make(chan *Packet, packetChannelMax),
		LastActive:     time.Now(),
		ID:             uuid,
		Alive:          true,
		SendBuffer:     &bytes.Buffer{},
		BufferMutex:    sync.Mutex{},
		OnClose:        onClose,
	}
	return tunnel
}

// 监听tcp数据
func (t *Tunnel) Listen() {
	Debug("[tunnel] start a listen goroutine")
	buffer := make([]byte, buffSize) // 设定一个合适的缓冲区
	for {
		select {
		case <-t.Done:
			Debug("[tunnel] closed a listen goroutine")
			// 如果收到关闭信号，退出 goroutine
			return
		default:
			// 读取数据
			n, err := t.Conn.Read(buffer)
			if err != nil {
				if err == io.EOF {
					//对方主动断开连接
					t.Close()
					return
				}
				if err != net.ErrClosed {
					Error("Failed to read data from connection [ID: %d]: %v", t.ID, err)
				}
				close(t.Done) // 关闭连接
				return
			}

			// 处理读取的数据
			if n > 0 {
				data := make([]byte, n)
				copy(data, buffer[:n])

				// Write data to send buffer
				t.BufferMutex.Lock()
				if t.SendBuffer != nil {
					_, err := t.SendBuffer.Write(data)
					if err != nil {
						Error("Failed to write to send buffer: %v", err)
					}
				}
				t.BufferMutex.Unlock()
			}
		}
	}
}

func (t *Tunnel) Close() {
	if t.Alive {
		err := t.Conn.Close()
		if err != nil {
			Error("Failed to close tunnel [ConnID: %d]: %v", t.ID, err)
			return
		}
		Debug("[tunnel] close a tcp connection [%v]", t.Conn.RemoteAddr())
		t.Alive = false
		if t.OnClose != nil {
			t.OnClose(t)
		}
	}
}

// 向tcp连接中写入数据
func (t *Tunnel) Write(data []byte) error {
	_, err := t.Conn.Write(data)
	if err != nil {
		WithFields(logrus.Fields{
			"error":  err,
			"connID": t.ID,
		}).Error("Failed to write data")
		return err
	}
	return nil
}
