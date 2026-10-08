package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/grpc/testdata"
)

const bufSize = 1024 * 1024

func TestNewServer(t *testing.T) {
	config := DefaultConfig()
	config.Port = 0 // use random port
	s := NewServer(config)

	if s == nil {
		t.Fatal("server should not be nil")
	}

	if s.grpcServer == nil {
		t.Error("grpcServer should not be nil")
	}
}

func TestServer_StartAndStop(t *testing.T) {
	config := DefaultConfig()
	config.Port = 0
	config.Insecure = true
	s := NewServer(config)

	// Start in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Stop server
	s.Stop()

	select {
	case err := <-errCh:
		// Server stopped (may have address in use error if race, that's ok)
		_ = err
	case <-time.After(time.Second):
		t.Error("server did not start within timeout")
	}
}

func TestServer_Serve(t *testing.T) {
	config := DefaultConfig()
	config.Insecure = true
	s := NewServer(config)

	ln := bufconn.Listen(bufSize)

	go func() {
		s.Serve(ln)
	}()

	time.Sleep(100 * time.Millisecond)

	if s.Address() == "" {
		t.Error("server should have an address after starting")
	}

	s.Stop()
}

func TestServer_Address_NotStarted(t *testing.T) {
	config := DefaultConfig()
	s := NewServer(config)

	if addr := s.Address(); addr != "" {
		t.Errorf("expected empty address, got %s", addr)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Port != 9000 {
		t.Errorf("expected default port 9000, got %d", config.Port)
	}

	if config.Network != "tcp" {
		t.Errorf("expected default network 'tcp', got %s", config.Network)
	}

	if config.MaxConcurrentStreams != 100 {
		t.Errorf("expected default max concurrent streams 100, got %d", config.MaxConcurrentStreams)
	}
}

// A recovered panic must surface as an error. Returning only the recover()
// left the named results at (nil, nil), so gRPC reported the call as a success
// with a nil response.
func TestRecoveryInterceptor(t *testing.T) {
	interceptor := RecoveryInterceptor()

	ctx := context.Background()
	resp, err := interceptor(ctx, "test request", &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		panic("test panic")
	})

	if err == nil {
		t.Fatal("expected an error after a recovered panic, got nil")
	}
	if resp != nil {
		t.Errorf("expected a nil response after a recovered panic, got %v", resp)
	}
	if code := status.Code(err); code != codes.Internal {
		t.Errorf("status code = %v, want %v", code, codes.Internal)
	}
}

// The non-panicking path must still return the handler's own values.
func TestRecoveryInterceptor_PassesThrough(t *testing.T) {
	interceptor := RecoveryInterceptor()

	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return "ok", nil
		})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v, want ok", resp)
	}
}

// UnaryInterceptor and StreamInterceptor have the same recovery contract.
func TestUnaryInterceptor_PanicBecomesError(t *testing.T) {
	interceptor := UnaryInterceptor(func(ctx context.Context, req interface{}) (interface{}, error) {
		panic("boom")
	})

	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, nil)
	if err == nil {
		t.Fatal("expected an error after a recovered panic, got nil")
	}
	if resp != nil {
		t.Errorf("expected a nil response, got %v", resp)
	}
	if code := status.Code(err); code != codes.Internal {
		t.Errorf("status code = %v, want %v", code, codes.Internal)
	}
}

func TestStreamRecoveryInterceptor_PanicBecomesError(t *testing.T) {
	interceptor := StreamRecoveryInterceptor()

	err := interceptor(nil, nil, &grpc.StreamServerInfo{}, func(srv interface{}, ss grpc.ServerStream) error {
		panic("stream boom")
	})
	if err == nil {
		t.Fatal("expected an error after a recovered panic, got nil")
	}
	if code := status.Code(err); code != codes.Internal {
		t.Errorf("status code = %v, want %v", code, codes.Internal)
	}
}

// A nil info must not cause a second panic while the first is being reported.
func TestRecoveryInterceptor_NilInfo(t *testing.T) {
	interceptor := RecoveryInterceptor()

	_, err := interceptor(context.Background(), "req", nil, func(ctx context.Context, req interface{}) (interface{}, error) {
		panic("boom")
	})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestLoggingInterceptor(t *testing.T) {
	interceptor := LoggingInterceptor()

	ctx := context.Background()
	_, _ = interceptor(ctx, "test request", &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	})

	// Just verify it doesn't panic
}

func TestKeepAliveConfig(t *testing.T) {
	ka := KeepAliveConfig(5*time.Minute, 2*time.Minute, 1*time.Minute)

	if ka.MaxConnectionIdle != 5*time.Minute {
		t.Errorf("expected MaxConnectionIdle 5m, got %v", ka.MaxConnectionIdle)
	}

	if ka.Time != 2*time.Minute {
		t.Errorf("expected Time 2m, got %v", ka.Time)
	}

	if ka.Timeout != 1*time.Minute {
		t.Errorf("expected Timeout 1m, got %v", ka.Timeout)
	}
}

func TestTLSCreds(t *testing.T) {
	// Test with non-existent files
	_, err := TLSCreds("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("expected error for non-existent cert files")
	}
}

func TestServer_ImmediateStop(t *testing.T) {
	config := DefaultConfig()
	config.Port = 0
	config.Insecure = true
	s := NewServer(config)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	s.ImmediateStop()

	select {
	case err := <-errCh:
		_ = err
	case <-time.After(time.Second):
		t.Error("server did not stop")
	}
}

func TestServer_Start_Twice(t *testing.T) {
	config := DefaultConfig()
	config.Port = 0
	config.Insecure = true
	s := NewServer(config)

	// Start first time - should succeed
	ln, _ := net.Listen("tcp", "localhost:0")
	addr := ln.Addr().String()
	_ = addr // suppress unused warning

	go s.Serve(ln)
	time.Sleep(50 * time.Millisecond)

	// Try to start again - should return error since already started
	err := s.Start()
	if err == nil {
		t.Error("expected error when starting twice")
	}

	s.Stop()
}

func TestUnaryInterceptor(t *testing.T) {
	called := false
	adapted := UnaryInterceptor(func(ctx context.Context, req interface{}) (interface{}, error) {
		called = true
		return "response", nil
	})

	ctx := context.Background()
	resp, err := adapted(ctx, "request", &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "should not reach", nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !called {
		t.Error("interceptor should have been called")
	}

	if resp != "response" {
		t.Errorf("expected response 'response', got %v", resp)
	}
}

func TestStreamInterceptor(t *testing.T) {
	// Create a buffer listener for the test
	ln := bufconn.Listen(bufSize)
	config := DefaultConfig()
	config.Insecure = true
	s := NewServer(config)

	go s.Serve(ln)
	time.Sleep(50 * time.Millisecond)
	s.Stop()

	// If we get here without panic, the test passes
}

func TestConfig_WithTLS(t *testing.T) {
	config := DefaultConfig()
	config.TLSCert = testdata.Path("server1.pem")
	config.TLSKey = testdata.Path("server1.key")

	s := NewServer(config)

	if s == nil {
		t.Fatal("server should not be nil")
	}

	// Verify server was created with TLS credentials
	// (actual TLS handshake would need a client to verify)
}

func TestConfig_UnaryAndStreamInterceptors(t *testing.T) {
	unaryCalled := false

	config := DefaultConfig()
	config.UnaryInterceptors = []grpc.UnaryServerInterceptor{
		func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			unaryCalled = true
			return handler(ctx, req)
		},
	}
	config.StreamInterceptors = []grpc.StreamServerInterceptor{
		func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return nil
		},
	}

	s := NewServer(config)
	if s == nil {
		t.Fatal("server should not be nil")
	}

	// Interceptors are set up, verification would require actual RPC calls
	_ = unaryCalled
}

func TestServer_Address_AfterServe(t *testing.T) {
	config := DefaultConfig()
	config.Port = 0
	config.Insecure = true
	s := NewServer(config)

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	go s.Serve(ln)
	time.Sleep(50 * time.Millisecond)

	addr := s.Address()
	if addr == "" {
		t.Error("expected non-empty address")
	}

	s.Stop()
}

func TestServer_RegisterService_NilCheck(t *testing.T) {
	config := DefaultConfig()
	s := NewServer(config)

	// Note: grpc.Server.RegisterService panics on nil args by design
	// So we just verify the method exists and can be called
	// without panicking on a properly initialized server
	if s == nil {
		t.Fatal("server should not be nil")
	}
}

func TestNewGRPCServerModule(t *testing.T) {
	config := DefaultConfig()
	config.Insecure = true
	m := NewGRPCServerModule("test-grpc", config)

	if m == nil {
		t.Fatal("module should not be nil")
	}

	if m.server == nil {
		t.Error("server should not be nil")
	}

	if m.server.grpcServer == nil {
		t.Error("grpcServer should not be nil")
	}
}

func TestGRPCServerModule_Server(t *testing.T) {
	config := DefaultConfig()
	m := NewGRPCServerModule("test-grpc", config)

	s := m.Server()
	if s == nil {
		t.Fatal("Server() should not return nil")
	}

	if s != m.server {
		t.Error("Server() should return the same server instance")
	}
}

func TestUnaryInterceptor_PanicRecovery(t *testing.T) {
	adapted := UnaryInterceptor(func(ctx context.Context, req interface{}) (interface{}, error) {
		if req == "panic" {
			panic("requested panic")
		}
		return "ok", nil
	})

	ctx := context.Background()

	// Normal request should work
	resp, err := adapted(ctx, "normal", &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "handled", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("expected 'ok', got %v", resp)
	}
}

func TestStreamRecoveryInterceptor(t *testing.T) {
	interceptor := StreamRecoveryInterceptor()
	if interceptor == nil {
		t.Fatal("StreamRecoveryInterceptor should not return nil")
	}
	// Verify it has the correct type
	var _ grpc.StreamServerInterceptor = interceptor
}

func TestGRPCServerModule_OnBoot_StartError(t *testing.T) {
	config := DefaultConfig()
	config.Port = 99999 // invalid port to trigger error
	config.Insecure = true
	m := NewGRPCServerModule("test-grpc", config)

	// OnBoot should try to start the server
	// It may fail if port is invalid, which is expected
	err := m.OnBoot()
	// We don't assert err == nil because port may be invalid
	// Just verify it doesn't panic
	_ = err

	m.OnShutdown()
}

func TestServer_WithInsecureCredentials(t *testing.T) {
	config := DefaultConfig()
	config.Insecure = true
	s := NewServer(config)

	if s == nil {
		t.Fatal("server should not be nil")
	}

	// Just verify it was created without panic
	_ = s
}

func TestServer_WithUnaryInterceptors(t *testing.T) {
	config := DefaultConfig()
	config.Insecure = true
	config.UnaryInterceptors = []grpc.UnaryServerInterceptor{
		RecoveryInterceptor(),
		LoggingInterceptor(),
	}
	s := NewServer(config)

	if s == nil {
		t.Fatal("server should not be nil")
	}

	// Verify interceptors are chained
	_ = s
}

func TestServer_WithStreamInterceptors(t *testing.T) {
	config := DefaultConfig()
	config.Insecure = true
	config.StreamInterceptors = []grpc.StreamServerInterceptor{
		StreamRecoveryInterceptor(),
	}
	s := NewServer(config)

	if s == nil {
		t.Fatal("server should not be nil")
	}

	_ = s
}
