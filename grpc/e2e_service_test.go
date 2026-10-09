package grpc

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/status"
)

// testCodecName is the content-subtype the e2e tests negotiate.
const testCodecName = "gofault-test-json"

type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error) { return json.Marshal(v) }

func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func (jsonCodec) Name() string { return testCodecName }

func init() { encoding.RegisterCodec(jsonCodec{}) }

// A hand-written service used by the end-to-end tests. The module has no .proto
// and generating one would add a toolchain dependency, so the messages are
// plain Go structs carried by a JSON codec. Everything above the wire -- service
// registration, the interceptor chain, status propagation, concurrency -- is the
// real gRPC implementation.

type EchoRequest struct {
	Message string
}

func (r *EchoRequest) GetMessage() string {
	if r == nil {
		return ""
	}
	return r.Message
}

type EchoReply struct {
	Message string
}

func (r *EchoReply) GetMessage() string {
	if r == nil {
		return ""
	}
	return r.Message
}

type echoServiceImpl struct {
	// failWith, when non-zero, is returned from Echo.
	failWith codes.Code
	// panicOnEcho makes Echo panic, to verify the recovery interceptors.
	panicOnEcho bool
}

func (s echoServiceImpl) Echo(ctx context.Context, req *EchoRequest) (*EchoReply, error) {
	if s.panicOnEcho {
		panic("echo handler exploded")
	}
	if s.failWith != 0 {
		return nil, status.Error(s.failWith, "rejected by test service")
	}
	return &EchoReply{Message: req.GetMessage()}, nil
}

// EchoServiceDesc describes the service to grpc's registry.
var EchoServiceDesc = grpc.ServiceDesc{
	ServiceName: "gofault.test.Echo",
	HandlerType: (*echoServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Echo",
			Handler: func(srv any, ctx context.Context, dec func(any) error,
				interceptor grpc.UnaryServerInterceptor) (any, error) {
				in := new(EchoRequest)
				if err := dec(in); err != nil {
					return nil, err
				}
				if interceptor == nil {
					return srv.(echoServiceServer).Echo(ctx, in)
				}
				info := &grpc.UnaryServerInfo{
					Server:     srv,
					FullMethod: "/gofault.test.Echo/Echo",
				}
				handler := func(ctx context.Context, req any) (any, error) {
					return srv.(echoServiceServer).Echo(ctx, req.(*EchoRequest))
				}
				return interceptor(ctx, in, info, handler)
			},
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "e2e_test.go",
}

type echoServiceServer interface {
	Echo(context.Context, *EchoRequest) (*EchoReply, error)
}

type echoClient struct {
	cc grpc.ClientConnInterface
}

// EchoClient is the client interface for the echo service.
type EchoClient interface {
	Echo(ctx context.Context, in *EchoRequest, opts ...grpc.CallOption) (*EchoReply, error)
}

// NewEchoClient builds a client over the given connection.
func NewEchoClient(cc grpc.ClientConnInterface) EchoClient {
	return &echoClient{cc: cc}
}

func (c *echoClient) Echo(ctx context.Context, in *EchoRequest, opts ...grpc.CallOption) (*EchoReply, error) {
	out := new(EchoReply)
	if err := c.cc.Invoke(ctx, "/gofault.test.Echo/Echo", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func errBadReply(got string) error {
	return fmt.Errorf("bad reply: %q", got)
}
