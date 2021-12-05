package emux

import (
	"context"
	"fmt"
	"net"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	bufSize = 32 * 1024
)

var (
	ErrClosed               = net.ErrClosed
	ErrTimeout              = fmt.Errorf("timeout")
	ErrAlreadyStarted       = fmt.Errorf("session already started")
	errUnknownStreamID      = fmt.Errorf("unknown stream id")
	errStreamIsAlreadyReady = fmt.Errorf("stream is already ready")
	errNoFreeStreamID       = fmt.Errorf("emux: no free stream id")
	errShortRead            = fmt.Errorf("read length not equal to body length")
	errStreamAlreadyExists  = fmt.Errorf("stream id already exists")
	errNoData               = fmt.Errorf("data length cannot be zero")
)

type Flusher interface {
	Buffered() int
	Flush() error
}

type ByteReader interface {
	ReadByte() (byte, error)
}

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

// isClosedConnError reports whether err is an error from use of a closed
// network connection.
func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}

	if err == ErrClosed {
		return true
	}

	str := err.Error()
	if strings.Contains(str, "use of closed network connection") {
		return true
	}

	if runtime.GOOS == "windows" {
		if oe, ok := err.(*net.OpError); ok && oe.Op == "read" {
			if se, ok := oe.Err.(*os.SyscallError); ok && se.Syscall == "wsarecv" {
				const WSAECONNABORTED = 10053
				const WSAECONNRESET = 10054
				if n := errno(se.Err); n == WSAECONNRESET || n == WSAECONNABORTED {
					return true
				}
			}
		}
	}
	return false
}

func errno(v error) uintptr {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Uintptr {
		return uintptr(rv.Uint())
	}
	return 0
}

type onceError struct {
	sync.Mutex // guards following
	err        error
}

func (a *onceError) Store(err error) {
	a.Lock()
	defer a.Unlock()
	if a.err != nil {
		return
	}
	a.err = err
}
func (a *onceError) Load() error {
	a.Lock()
	defer a.Unlock()
	return a.err
}
