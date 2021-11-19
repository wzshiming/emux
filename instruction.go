package emux

import (
	"encoding/binary"
	"time"
)

// Frame
//
// Stream                                                  Stream
//        1 ------|\                            /|------ 1
//        2 ------| \                          / |------ 2
//        3 ------|  >--- Frame On Stream ----<  |------ 3
//        4 ------| /                          \ |------ 4
//        5 ------|/                            \|------ 5

type Instruction struct {

	// 0         1
	// +---------+
	// | Command |
	// +---------+

	Close uint8 // close all sessions and connections, only command
	Ping  uint8 // ping, only command
	Pong  uint8 // reply to ping, only command

	// 0         1         (StreamID Length + 1)
	// +---------+---------+
	// | Command | StreamID|
	// +---------+---------+

	Connect      uint8 // create a connect stream, with both command and stream id
	Connected    uint8 // reply to connect, with both command and stream id
	Disconnect   uint8 // disconnect a stream and report the reason, with both command and stream id
	Disconnected uint8 // reply to disconnect, with both command and stream id

	// 0         1         (StreamID Length + 1)                   (Frame length + StreamID Length + 1)
	// +---------+---------+---------+---------+---------+---------+
	// | Command | StreamID|   Data length and data packet ...     |
	// +---------+---------+---------+---------+---------+---------+

	Data uint8 // data packet, all fields

	MaxDataPacketSize uint64 // max data packet size

	HeartbeatInterval time.Duration // heartbeat interval
}

var DefaultInstruction = Instruction{
	Close:             0x00,
	Ping:              0xee,
	Pong:              0xef,
	Connect:           0xa0,
	Connected:         0xa1,
	Disconnect:        0xc0,
	Disconnected:      0xc1,
	Data:              0xb0,
	MaxDataPacketSize: bufSize - 1 - 2*binary.MaxVarintLen64,
	HeartbeatInterval: 10 * time.Second,
}
