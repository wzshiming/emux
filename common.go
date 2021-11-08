package emux

import (
	"context"
	"net"
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

type BytesPool interface {
	Get() []byte
	Put([]byte)
}
