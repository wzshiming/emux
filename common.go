package emux

import (
	"context"
	"net"
	"time"
)

const (
	bufSize = 32 * 1024
)

// ListenConfig contains options for listening to an address.
type ListenConfig interface {
	Listen(ctx context.Context, network, address string) (net.Listener, error)
}

// Dialer contains options for connecting to an address.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Logger is the interface for logging.
type Logger interface {
	Println(v ...interface{})
}

// BytesPool is the interface for a bytes pool.
type BytesPool interface {
	Get() []byte
	Put([]byte)
}

type deadline interface {
	SetDeadline(t time.Time) error
}
