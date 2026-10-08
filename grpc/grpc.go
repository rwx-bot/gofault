package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	"github.com/gofault/gofault/core"
)

// Config holds gRPC server configuration.
type Config struct {
	// Port to listen on (default 9000).
	Port int
	// Network type (tcp, tcp4, tcp6).
	Network string
	// KeepAlive server parameters.
	KeepAlive *keepalive.ServerParameters
	// MaxConcurrentStreams sets the limit on the number of concurrent streams.
	MaxConcurrentStreams uint32
	// Insecure skips TLS (default false).
	Insecure bool
	// TLSCert and TLSKey for TLS credentials.
	TLSCert string
	TLSKey  string
	// UnaryInterceptors registers unary server interceptors.
	UnaryInterceptors []grpc.UnaryServerInterceptor
	// StreamInterceptors registers stream server interceptors.
	StreamInterceptors []grpc.StreamServerInterceptor
}

// DefaultConfig returns a default gRPC configuration.
func DefaultConfig() Config {
	return Config{
		Port:                 9000,
		Network:              "tcp",
		MaxConcurrentStreams: 100,
	}
}

// Server represents a gRPC server.
type Server struct {
	config     Config
	grpcServer *grpc.Server
	listener   net.Listener
	mu         sync.Mutex
	started    bool
}

// NewServer creates a new gRPC server.
func NewServer(config Config) *Server {
	if config.Port == 0 {
		config.Port = 9000
	}
	if config.Network == "" {
		config.Network = "tcp"
	}

	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(config.MaxConcurrentStreams),
	}

	// Build interceptors chain
	if len(config.UnaryInterceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(config.UnaryInterceptors...))
	}
	if len(config.StreamInterceptors) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(config.StreamInterceptors...))
	}

	if config.KeepAlive != nil {
		opts = append(opts, grpc.KeepaliveParams(*config.KeepAlive))
	}

	var creds credentials.TransportCredentials
	if config.Insecure {
		creds = insecure.NewCredentials()
	} else if config.TLSCert != "" && config.TLSKey != "" {
		var err error
		creds, err = credentials.NewServerTLSFromFile(config.TLSCert, config.TLSKey)
		if err != nil {
			panic(fmt.Sprintf("failed to load TLS certs: %v", err))
		}
	}

	if creds != nil {
		opts = append(opts, grpc.Creds(creds))
	}

	return &Server{
		config:     config,
		grpcServer: grpc.NewServer(opts...),
	}
}

// RegisterService registers a gRPC service.
func (s *Server) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	s.grpcServer.RegisterService(desc, impl)
}

// Start listening on the configured port.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("server already started")
	}

	addr := fmt.Sprintf("%s:%d", s.config.Network, s.config.Port)
	ln, err := net.Listen(s.config.Network, addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.listener = ln
	s.started = true
	s.mu.Unlock()

	return s.grpcServer.Serve(ln)
}

// Serve starts the gRPC server on the given listener.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("server already started")
	}
	s.listener = ln
	s.started = true
	s.mu.Unlock()

	return s.grpcServer.Serve(ln)
}

// Stop gracefully shuts down the gRPC server.
func (s *Server) Stop() {
	s.grpcServer.GracefulStop()
}

// ImmediateStop immediately shuts down the gRPC server.
func (s *Server) ImmediateStop() {
	s.grpcServer.Stop()
}

// Address returns the listening address, or empty string if not started.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// logRPCPanic records a recovered panic. Every interceptor here must convert a
// panic into an error: recovering without returning one leaves the named return
// values at their zero value, so gRPC would report the call as a success with a
// nil response and no error.
func logRPCPanic(method, kind string, r interface{}) {
	slog.Default().Error("grpc panic recovered",
		"kind", kind, "method", method, "panic", r, "stack", string(debug.Stack()))
}

// panicError converts a recovered panic into a gRPC error.
func panicError(r interface{}) error {
	return status.Errorf(codes.Internal, "panic recovered: %v", r)
}

// methodName extracts the full method name, tolerating a nil info.
func methodName(fullMethod string) string { return fullMethod }

// UnaryInterceptor adapts a function to grpc.UnaryServerInterceptor.
//
// The adapter recovers from panics in fn and reports them as Internal errors.
// It intentionally does not invoke the next handler: fn is the whole operation.
// Chain RecoveryInterceptor or LoggingInterceptor separately when they are
// needed alongside it.
func UnaryInterceptor(fn func(ctx context.Context, req interface{}) (interface{}, error)) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logRPCPanic(methodName(infoFullMethod(info)), "unary", r)
				resp, err = nil, panicError(r)
			}
		}()
		return fn(ctx, req)
	}
}

// StreamInterceptor adapts a function to grpc.StreamServerInterceptor.
// A panic in fn is recovered and returned as an Internal error.
func StreamInterceptor(fn func(srv interface{}, ss grpc.ServerStream) error) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logRPCPanic(methodName(infoFullMethodStream(info)), "stream", r)
				err = panicError(r)
			}
		}()
		return fn(srv, ss)
	}
}

// RecoveryInterceptor returns a unary interceptor that recovers from panics in
// the handler and reports them as Internal errors.
func RecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logRPCPanic(methodName(infoFullMethod(info)), "unary", r)
				resp, err = nil, panicError(r)
			}
		}()
		return handler(ctx, req)
	}
}

// StreamRecoveryInterceptor returns a stream interceptor that recovers from
// panics in the handler and reports them as Internal errors.
func StreamRecoveryInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logRPCPanic(methodName(infoFullMethodStream(info)), "stream", r)
				err = panicError(r)
			}
		}()
		return handler(srv, ss)
	}
}

// infoFullMethod and infoFullMethodStream read the method name defensively:
// gRPC always supplies info, but a nil one must not mask a panic being reported.
func infoFullMethod(info *grpc.UnaryServerInfo) string {
	if info == nil {
		return ""
	}
	return info.FullMethod
}

func infoFullMethodStream(info *grpc.StreamServerInfo) string {
	if info == nil {
		return ""
	}
	return info.FullMethod
}

// LoggingInterceptor returns a unary interceptor that logs requests.
func LoggingInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)

		attrs := []any{
			"method", infoFullMethod(info),
			"duration", time.Since(start),
			"error", err,
		}
		if err != nil {
			slog.Default().Warn("grpc request failed", attrs...)
		} else {
			slog.Default().Info("grpc request", attrs...)
		}
		return resp, err
	}
}

// KeepAliveConfig creates keepalive.ServerParameters.
func KeepAliveConfig(maxIdle, pingAfter, minTime time.Duration) keepalive.ServerParameters {
	return keepalive.ServerParameters{
		MaxConnectionIdle: maxIdle,
		Time:              pingAfter,
		Timeout:           minTime,
	}
}

// TLSCreds creates TLS credentials from certificate files.
func TLSCreds(certFile, keyFile string) (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
	}), nil
}

// GRPCServerModule wraps Server as a core.Module for integration with gofault.
type GRPCServerModule struct {
	*Module
	server *Server
}

// NewGRPCServerModule creates a new gRPC server module.
func NewGRPCServerModule(name string, config Config) *GRPCServerModule {
	return &GRPCServerModule{
		Module: core.NewModule(name),
		server: NewServer(config),
	}
}

// Server returns the underlying *Server.
func (m *GRPCServerModule) Server() *Server {
	return m.server
}

// OnBoot starts the gRPC server.
func (m *GRPCServerModule) OnBoot() error {
	return m.server.Start()
}

// OnShutdown gracefully stops the gRPC server.
func (m *GRPCServerModule) OnShutdown() error {
	m.server.Stop()
	return nil
}

// Module is an alias for core.Module to avoid import cycle.
type Module = core.Module
