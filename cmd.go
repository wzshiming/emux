package emux

//go:generate stringer -type=Cmd
type Cmd uint8

// Cmd Frame
//
// Stream                                                  Stream
//        1 ------|\                            /|------ 1
//        2 ------| \                          / |------ 2
//        3 ------|  >--- Frame On Stream ----<  |------ 3
//        4 ------| /                          \ |------ 4
//        5 ------|/                            \|------ 5

const (

	// 0         1
	// +---------+
	// | Command |
	// +---------+

	CmdClose Cmd = 0

	// 0         1         (StreamID Length + 1)
	// +---------+---------+
	// | Command | StreamID|
	// +---------+---------+

	CmdConnect      Cmd = 0xa0 // create a connect stream, with both command and stream id
	CmdConnected    Cmd = 0xa1 // reply to connect, with both command and stream id
	CmdDisconnect   Cmd = 0xc0 // disconnect a stream and report the reason, with both command and stream id
	CmdDisconnected Cmd = 0xc1 // reply to disconnect, with both command and stream id

	// 0         1         (StreamID Length + 1)                   (Frame length + StreamID Length + 1)
	// +---------+---------+---------+---------+---------+---------+
	// | Command | StreamID|   Data length and data packet ...     |
	// +---------+---------+---------+---------+---------+---------+

	CmdData Cmd = 0xb0 // data packet, all fields
)
