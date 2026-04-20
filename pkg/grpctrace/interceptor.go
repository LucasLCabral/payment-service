package grpctrace

import (
	"context"

	"github.com/LucasLCabral/payment-service/pkg/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		tid := trace.NewTraceID()
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if v := md.Get(trace.XTraceIDHeader); len(v) > 0 && v[0] != "" {
				tid = v[0]
			}
		}
		ctx = trace.WithTraceID(ctx, tid)
		return handler(ctx, req)
	}
}

func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		tid := trace.GetTraceID(ctx)
		ctx = metadata.AppendToOutgoingContext(ctx, trace.XTraceIDHeader, tid)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
