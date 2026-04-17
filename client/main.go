package main

import (
	"context"
	"flag"
	"log"
	"time"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/metadata"

	pb "github.com/ahl1u/gRPC-fault-tolerance-monitor/proto"
)

var (
	addr = flag.String("addr", "localhost:50051", "the address to connect to")
)

const maxCalls = 3

func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error
		var header metadata.MD
		for attempt := uint(0); attempt < maxCalls; attempt++ {
			lastErr = invoker(ctx, method, req, reply, cc, append(opts, grpc.Header(&header))...)
			if lastErr == nil {
				return nil
			}

			st, ok := status.FromError(lastErr)
			if !ok {
				return lastErr
			}

			switch st.Code() {
			case codes.Unavailable:
				vals := header["leader-addr"]
				if len(vals) > 0 {
					newAddr := vals[0]
					conn, err := grpc.NewClient(newAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
					if err != nil {
						return err
					}
					defer conn.Close()
					return invoker(ctx, method, req, reply, conn, opts...)
				}
				continue
			default:
				return lastErr
			}
		}
		return lastErr

	}
}


func main() {
	flag.Parse()
	// Set up a connection to the server.
	conn, err := grpc.NewClient(*addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientInterceptor()),
	)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewFaultTolerantClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
    resp, err := c.Execute(ctx, &pb.Request{Id: "1", Payload: "mr kim"})
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	log.Printf("response: %v", resp.GetResult())
}