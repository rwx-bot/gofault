package grpc

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// TestServer_EndToEndUnary drives a real gRPC client against a registered
// service. RegisterService had no coverage at all, and the test that claimed to
// exercise it never called it, so the whole service registration path was
// unverified.
func TestServer_EndToEndUnary(t *testing.T) {
	ln := bufconn.Listen(bufSize)
	s := NewServer(DefaultConfig())
	s.RegisterService(&EchoServiceDesc, echoServiceImpl{})

	served := make(chan error, 1)
	go func() { served <- s.Serve(ln) }()
	// GracefulStop waits for every open stream, which does not settle over
	// bufconn; the dedicated Stop tests cover the graceful path.
	t.Cleanup(func() {
		s.ImmediateStop()
		<-served
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return ln.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype(testCodecName)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	client := NewEchoClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reply, err := client.Echo(ctx, &EchoRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("Echo: %v", err)
	}
	if reply.GetMessage() != "hello" {
		t.Errorf("reply = %q, want %q", reply.GetMessage(), "hello")
	}
}

// A handler returning an error must reach the client with the right status code,
// which is what the interceptors exist to preserve.
func TestServer_EndToEndHandlerError(t *testing.T) {
	ln := bufconn.Listen(bufSize)
	s := NewServer(DefaultConfig())
	s.RegisterService(&EchoServiceDesc, echoServiceImpl{failWith: codes.PermissionDenied})

	served := make(chan error, 1)
	go func() { served <- s.Serve(ln) }()
	// GracefulStop waits for every open stream, which does not settle over
	// bufconn; the dedicated Stop tests cover the graceful path.
	t.Cleanup(func() {
		s.ImmediateStop()
		<-served
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return ln.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype(testCodecName)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = NewEchoClient(conn).Echo(ctx, &EchoRequest{Message: "x"})
	if err == nil {
		t.Fatal("expected an error from the handler")
	}
	if code := status.Code(err); code != codes.PermissionDenied {
		t.Errorf("status = %v, want %v", code, codes.PermissionDenied)
	}
}

// A panicking handler must not take the server down, and the client must see an
// Internal error. This is the whole point of the recovery interceptors.
func TestServer_EndToEndPanicRecovered(t *testing.T) {
	ln := bufconn.Listen(bufSize)

	cfg := DefaultConfig()
	cfg.UnaryInterceptors = []grpc.UnaryServerInterceptor{RecoveryInterceptor()}
	s := NewServer(cfg)
	s.RegisterService(&EchoServiceDesc, echoServiceImpl{panicOnEcho: true})

	served := make(chan error, 1)
	go func() { served <- s.Serve(ln) }()
	// GracefulStop waits for every open stream, which does not settle over
	// bufconn; the dedicated Stop tests cover the graceful path.
	t.Cleanup(func() {
		s.ImmediateStop()
		<-served
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return ln.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype(testCodecName)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = NewEchoClient(conn).Echo(ctx, &EchoRequest{Message: "x"})
	if err == nil {
		t.Fatal("expected an error from the panicking handler")
	}
	if code := status.Code(err); code != codes.Internal {
		t.Errorf("status = %v, want %v", code, codes.Internal)
	}
}

// Serve must reject a second attempt, so a double Start cannot leave two
// goroutines serving the same listener.
func TestServer_ServeIsNotReusable(t *testing.T) {
	ln := bufconn.Listen(bufSize)
	s := NewServer(DefaultConfig())

	served := make(chan error, 1)
	go func() { served <- s.Serve(ln) }()
	// GracefulStop waits for every open stream, which does not settle over
	// bufconn; the dedicated Stop tests cover the graceful path.
	t.Cleanup(func() {
		s.ImmediateStop()
		<-served
	})

	// Give the first Serve a moment to take the listener.
	time.Sleep(50 * time.Millisecond)

	other := bufconn.Listen(bufSize)
	defer other.Close()
	if err := s.Serve(other); err == nil {
		t.Error("expected Serve to refuse a second call")
	}
}

// Concurrent clients must all be served, which exercises the interceptor chain
// under real concurrency rather than a single call.
func TestServer_ConcurrentClients(t *testing.T) {
	ln := bufconn.Listen(bufSize)
	cfg := DefaultConfig()
	cfg.UnaryInterceptors = []grpc.UnaryServerInterceptor{RecoveryInterceptor(), LoggingInterceptor()}
	s := NewServer(cfg)
	s.RegisterService(&EchoServiceDesc, echoServiceImpl{})

	served := make(chan error, 1)
	go func() { served <- s.Serve(ln) }()
	// GracefulStop waits for every open stream, which does not settle over
	// bufconn; the dedicated Stop tests cover the graceful path.
	t.Cleanup(func() {
		s.ImmediateStop()
		<-served
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return ln.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.CallContentSubtype(testCodecName)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	client := NewEchoClient(conn)

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			reply, err := client.Echo(ctx, &EchoRequest{Message: "ping"})
			if err != nil {
				errs <- err
				return
			}
			if reply.GetMessage() != "ping" {
				errs <- errBadReply(reply.GetMessage())
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent call: %v", err)
	}
}
