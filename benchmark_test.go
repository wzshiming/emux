package emux

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkEmux(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
	}))
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		b.Fatal(err)
	}
	svc.Listener = NewListener(context.Background(), l)
	svc.Start()
	defer svc.Close()

	dialer := NewDialer(&net.Dialer{})
	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: true,
		DialContext:       dialer.DialContext,
	}
	for n := 0; n != b.N; n++ {
		resp, err := cli.Get(svc.URL)
		if err != nil {
			b.Fatal(err)
		}
		if resp.StatusCode != 200 {
			b.Fatal("expected 200, got", resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func BenchmarkOrigin(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
	}))
	svc.Start()
	defer svc.Close()

	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: true,
	}
	for n := 0; n != b.N; n++ {
		resp, err := cli.Get(svc.URL)
		if err != nil {
			b.Fatal(err)
		}
		if resp.StatusCode != 200 {
			b.Fatal("expected 200, got", resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func BenchmarkParallelEmux(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
	}))
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		b.Fatal(err)
	}
	listener := NewListener(context.Background(), l)
	svc.Listener = listener
	svc.Start()
	defer svc.Close()

	dialer := NewDialer(&net.Dialer{})
	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: true,
		DialContext:       dialer.DialContext,
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := cli.Get(svc.URL)
			if err != nil {
				b.Fatal(err)
			}
			if resp.StatusCode != 200 {
				b.Fatal("expected 200, got", resp.StatusCode)
			}
			resp.Body.Close()
		}
	})
}

func BenchmarkParallelOrigin(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
	}))
	svc.Start()
	defer svc.Close()

	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: true,
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := cli.Get(svc.URL)
			if err != nil {
				b.Fatal(err)
			}
			if resp.StatusCode != 200 {
				b.Fatal("expected 200, got", resp.StatusCode)
			}
			resp.Body.Close()
		}
	})
}

func BenchmarkParallelEmuxStream(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		for i := 0; i < 10; i++ {
			writer.Write([]byte("hello world"))
		}
	}))
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		b.Fatal(err)
	}
	listener := NewListener(context.Background(), l)
	svc.Listener = listener
	svc.Start()
	defer svc.Close()

	dialer := NewDialer(&net.Dialer{})
	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: true,
		DialContext:       dialer.DialContext,
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := cli.Get(svc.URL)
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

func BenchmarkParallelOriginStream(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		for i := 0; i < 10; i++ {
			writer.Write([]byte("hello world"))
		}
	}))
	svc.Start()
	defer svc.Close()

	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: true,
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resp, err := cli.Get(svc.URL)
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
