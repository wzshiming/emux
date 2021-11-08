package emux

import (
	"context"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func BenchmarkSerialEmux(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
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

func BenchmarkSerialOrigin(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
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

func BenchmarkSerialOriginKeepAlives(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
	}))
	svc.Start()
	defer svc.Close()

	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: false,
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
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
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
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
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

func BenchmarkParallelOriginKeepAlives(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
	}))
	svc.Start()
	defer svc.Close()

	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: false,
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
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		io.Copy(rw, io.LimitReader(rand.Reader, 1024*1024))
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
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		io.Copy(rw, io.LimitReader(rand.Reader, 1024*1024))
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

func BenchmarkParallelOriginStreamKeepAlives(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		io.Copy(rw, io.LimitReader(rand.Reader, 1024*1024))
	}))
	svc.Start()
	defer svc.Close()

	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: false,
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

func BenchmarkParallelEmuxMicroPacketStream(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		io.CopyBuffer(rw, io.LimitReader(rand.Reader, 1024*1024), make([]byte, 10))
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

func BenchmarkParallelOriginMicroPacketStream(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		io.CopyBuffer(rw, io.LimitReader(rand.Reader, 1024*1024), make([]byte, 10))
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

func BenchmarkParallelOriginMicroPacketStreamKeepAlives(b *testing.B) {
	svc := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		io.CopyBuffer(rw, io.LimitReader(rand.Reader, 1024*1024), make([]byte, 10))
	}))
	svc.Start()
	defer svc.Close()

	cli := svc.Client()
	cli.Transport = &http.Transport{
		DisableKeepAlives: false,
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
