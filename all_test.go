package emux

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEmux(t *testing.T) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
	}))

	listenConfig := NewListenConfig(&net.ListenConfig{})
	l, err := listenConfig.Listen(context.Background(), "tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	listener := l
	svc.Listener = listener
	svc.Start()

	dialer := NewDialer(&net.Dialer{})

	cli := svc.Client()
	cli.Transport = &http.Transport{
		DialContext: dialer.DialContext,
	}
	resp, err := cli.Get(svc.URL)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatal("expected 200, got", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestTimeout(t *testing.T) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(time.Second * 2)
		writer.WriteHeader(200)
	}))
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	listener := NewListener(context.Background(), l)
	svc.Listener = listener
	svc.Start()

	dialer := NewDialer(&net.Dialer{})

	cli := svc.Client()
	cli.Transport = &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			conn.SetDeadline(time.Now().Add(time.Second))
			return conn, nil
		},
	}
	_, err = cli.Get(svc.URL)

	if err == nil || !errors.Is(err, ErrTimeout) {
		t.Fatal("expected Timeout, got", err)
	}
}
