package emux

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"testing"
)

func TestSession(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			sess := NewSession(conn)
			go func() {
				defer sess.Close()
				for {
					stm, err := sess.Accept(context.Background())
					if err != nil {
						if strings.Contains(err.Error(), "use of closed network connection") {
							return
						}
						t.Fatal(err)
					}
					buf := make([]byte, math.MaxUint16)
					go func() {
						for {
							n, err := stm.Read(buf)
							if err != nil {
								if err == io.EOF {
									return
								}
								t.Fatal(err)
							}
							_, err = stm.Write([]byte("echo " + string(buf[:n])))
							if err != nil {
								if strings.Contains(err.Error(), "use of closed network connection") {
									return
								}
								t.Fatal(err)
							}
						}
					}()
				}
			}()
		}
	}()

	buf := make([]byte, math.MaxUint16)
	for i := 0; i != 5; i++ {
		conn, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		sess := NewSession(conn)
		_, err = sess.Open(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		stm, err := sess.Open(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		for j := 0; j != 5; j++ {
			msg := fmt.Sprintf("hello %d %d !!!!!!", i, j)
			_, err := stm.Write([]byte(msg))
			if err != nil {
				t.Fatal(err)
			}
			n, err := stm.Read(buf)
			if err != nil {
				t.Fatal(err)
			}
			if string(buf[:n]) != "echo "+msg {
				t.Fatalf("fail trans: want %q, got %q", "echo "+msg, buf[:n])
			}
			t.Log(string(buf[:n]))
		}
		sess.Close()
	}
}
