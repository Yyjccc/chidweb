package common

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// PacketType 定义数据包类型
type PacketType uint8

const (
	PacketTypeHeartbeat PacketType = iota
	PacketTypeConnect
	PacketTypeData
	PacketTypeDisconnect
	PacketTypeError
)

// PacketHeader 包头格式
// +----------------+----------------+----------------+----------------+
// |  Magic Number  |  Packet Type  |   Client ID    |    Conn ID    |
// |    (2 bytes)   |    (1 byte)   |    (8 bytes)   |   (4 bytes)   |
// +----------------+----------------+----------------+----------------+
// |    TargetID    |   Sequence    |	  Length 	 |
// |    (4 bytes)   |   (4 bytes)   |	 (4 bytes)	 |
// +----------------+---------------+----------------+
const (
	MagicNumber    = 0x8086      // 魔数，用于验证包的有效性
	HeaderSize     = 27          // 包头大小
	MaxPayloadSize = 1024 * 1024 // 最大负载大小 1MB
)

// Packet 数据包结构
type Packet struct {
	Type      PacketType
	ClientID  uint64
	ChannelID uint32
	TargetID  uint32
	Sequence  uint32
	Length    uint32
	Payload   []byte
}

// Encode 将数据包编码为字节流
func (p *Packet) Encode() ([]byte, error) {
	if p.Length > MaxPayloadSize {
		return nil, errors.New("payload too large")
	}

	totalSize := HeaderSize + p.Length
	buf := make([]byte, totalSize)

	// 写入魔数
	binary.BigEndian.PutUint16(buf[0:2], MagicNumber)

	// 写入包类型
	buf[2] = byte(p.Type)

	// 写入ClientID
	binary.BigEndian.PutUint64(buf[3:11], p.ClientID)

	// 写入ChannelID
	binary.BigEndian.PutUint32(buf[11:15], p.ChannelID)

	//写入TargetID
	binary.BigEndian.PutUint32(buf[15:19], p.ChannelID)

	// 写入序列号
	binary.BigEndian.PutUint32(buf[19:23], p.Sequence)

	// 写入数据长度
	binary.BigEndian.PutUint32(buf[23:27], p.Length)

	// 写入负载数据
	if p.Length > 0 {
		copy(buf[HeaderSize:], p.Payload)
	}

	return buf, nil
}

// Decode 从字节流解码数据包
func DecodePacket(r io.Reader) (*Packet, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	// 验证魔数
	magic := binary.BigEndian.Uint16(header[0:2])
	if magic != MagicNumber {
		return nil, errors.New("invalid magic number")
	}

	p := &Packet{
		Type:      PacketType(header[2]),
		ClientID:  binary.BigEndian.Uint64(header[3:11]),
		ChannelID: binary.BigEndian.Uint32(header[11:15]),
		TargetID:  binary.BigEndian.Uint32(header[15:19]),
		Sequence:  binary.BigEndian.Uint32(header[19:23]),
		Length:    binary.BigEndian.Uint32(header[23:27]),
	}

	// 验证负载长度
	if p.Length > MaxPayloadSize {
		return nil, errors.New("payload too large")
	}

	// 读取负载数据
	if p.Length > 0 {
		p.Payload = make([]byte, p.Length)
		if _, err := io.ReadFull(r, p.Payload); err != nil {
			return nil, err
		}
	}

	return p, nil
}

// DecodePackets 从字节流解码多个数据包
func DecodePackets(data []byte) ([]*Packet, error) {
	var packets []*Packet
	reader := bytes.NewReader(data)

	for reader.Len() > 0 {
		packet, err := DecodePacket(reader)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		packets = append(packets, packet)
	}

	return packets, nil
}

// EncodePackets encodes multiple packets into a single byte slice
func EncodePackets(packets []*Packet) ([]byte, error) {
	var buf bytes.Buffer
	for _, packet := range packets {
		data, err := packet.Encode()
		if err != nil {
			return nil, err
		}
		if _, err := buf.Write(data); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
