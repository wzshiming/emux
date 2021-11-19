package emux

import (
	"context"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func BenchmarkAll(b *testing.B) {
	for _, serverFunc := range serverCases {
		for _, aliveFunc := range aliveCases {
			for _, wayFunc := range wayCases {
				for _, connFunc := range connCases {
					func() {
						serverName, server := serverFunc(b)
						aliveName, alive := aliveFunc(b)
						wayName, way := wayFunc(b)
						connName, dialer, listener := connFunc(b)
						server.Listener = listener
						server.Start()
						defer server.Close()
						cli := server.Client()
						cli.Transport = &http.Transport{
							DisableKeepAlives: alive,
							DialContext:       dialer.DialContext,
						}
						name := strings.Join([]string{serverName, aliveName, wayName, connName}, "-")
						b.Run(name, func(b *testing.B) {
							way(b, cli, server)
						})
					}()
				}
			}
		}
	}
}

var wayCases = []func(b *testing.B) (string, func(b *testing.B, cli *http.Client, server *httptest.Server)){
	func(b *testing.B) (string, func(b *testing.B, cli *http.Client, server *httptest.Server)) {
		return "serial", func(b *testing.B, cli *http.Client, server *httptest.Server) {
			for n := 0; n != b.N; n++ {
				resp, err := cli.Get(server.URL)
				if err != nil {
					b.Fatal(err)
				}
				if resp.StatusCode != 200 {
					b.Fatal("expected 200, got", resp.StatusCode)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
	},
	func(b *testing.B) (string, func(b *testing.B, cli *http.Client, server *httptest.Server)) {
		return "parallel", func(b *testing.B, cli *http.Client, server *httptest.Server) {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					resp, err := cli.Get(server.URL)
					if err != nil {
						b.Fatal(err)
					}
					if resp.StatusCode != 200 {
						b.Fatal("expected 200, got", resp.StatusCode)
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
			})
		}
	},
}

var aliveCases = []func(b *testing.B) (string, bool){
	func(b *testing.B) (string, bool) {
		return "disableKeepAlives", true
	},
	func(b *testing.B) (string, bool) {
		return "keepAlives", false
	},
}

var serverCases = []func(b *testing.B) (string, *httptest.Server){
	func(b *testing.B) (string, *httptest.Server) {
		return "nobody", httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(200)
		}))
	},
	func(b *testing.B) (string, *httptest.Server) {
		return "body-1<<10", httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			io.Copy(rw, io.LimitReader(rand.Reader, 1<<10))
		}))
	},
	func(b *testing.B) (string, *httptest.Server) {
		return "body-1<<20", httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			io.Copy(rw, io.LimitReader(rand.Reader, 1<<20))
		}))
	},
	func(b *testing.B) (string, *httptest.Server) {
		return "body-1<<30", httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			io.Copy(rw, io.LimitReader(rand.Reader, 1<<30))
		}))
	},
}

var connCases = []func(b *testing.B) (string, Dialer, net.Listener){
	func(b *testing.B) (string, Dialer, net.Listener) {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		l := &net.ListenConfig{}
		t, err := l.Listen(ctx, "tcp", ":0")
		if err != nil {
			b.Fatal(err)
		}
		return "tcpHttp", &net.Dialer{}, t
	},
	func(b *testing.B) (string, Dialer, net.Listener) {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		l := &net.ListenConfig{}
		t, err := l.Listen(ctx, "tcp", ":0")
		if err != nil {
			b.Fatal(err)
		}
		return "tcpHttpEmux", NewDialer(ctx, &net.Dialer{}), NewListener(ctx, t)
	},
	func(b *testing.B) (string, Dialer, net.Listener) {
		l := newTestPipeServer()
		return "pipeHttp", l, l
	},
	func(b *testing.B) (string, Dialer, net.Listener) {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		l := newTestPipeServer()
		return "pipeHttpEmux", NewDialer(ctx, l), NewListener(ctx, l)
	},
}

type testPipeServer struct {
	accept  chan net.Conn
	addr    net.Addr
	once    sync.Once
	isClose bool
}

func newTestPipeServer() *testPipeServer {
	return &testPipeServer{
		accept: make(chan net.Conn, 1),
		addr:   &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80},
	}
}

func (t *testPipeServer) Accept() (net.Conn, error) {
	conn, ok := <-t.accept
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (t *testPipeServer) Close() error {
	t.once.Do(func() {
		t.isClose = true
		close(t.accept)
	})
	return nil
}

func (t *testPipeServer) Addr() net.Addr {
	return t.addr
}

func (t *testPipeServer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if t.isClose {
		return nil, net.ErrClosed
	}
	c1, c2 := net.Pipe()
	t.accept <- c1
	return c2, nil
}
