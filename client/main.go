package main

import (
	"context"
	"flag"
	"io"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/ahl1u/gRPC-fault-tolerance-monitor/proto"
)

var (
	addr = flag.String("addr", "localhost:50051", "the address to connect to")
	mode = flag.String("mode", "unary", "unary or stream")
)

const maxCalls = 3

func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error
		var header metadata.MD

		for attempt := uint(0); attempt < maxCalls; attempt++ {
			// reset header each attempt
			header = metadata.MD{}
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
					// server told us who the leader is, redirect
					newAddr := vals[0]
					log.Printf("redirecting to leader at %s (attempt %d)", newAddr, attempt+1)
					conn, err := grpc.NewClient(newAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
					if err != nil {
						return err
					}
					defer conn.Close()
					// update cc for future retries too
					cc = conn
					continue // retry with new connection instead of returning immediately
				}
				// no redirect hint, wait briefly and retry same server
				log.Printf("unavailable, no redirect hint, retrying (attempt %d)", attempt+1)
				time.Sleep(100 * time.Millisecond)
				continue
			default:
				return lastErr
			}
		}
		return lastErr
	}
}

type retryStream struct {
	grpc.ClientStream
	ctx      context.Context
	desc     *grpc.StreamDesc
	cc       *grpc.ClientConn
	method   string
	streamer grpc.Streamer
	opts     []grpc.CallOption
}

func (s *retryStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	if st.Code() == codes.Unavailable {
		newStream, err := s.streamer(s.ctx, s.desc, s.cc, s.method, s.opts...)
		if err != nil {
			return err
		}
		s.ClientStream = newStream
		return nil
	}
	return err
}

func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			return nil, err
		}
		return &retryStream{
			ClientStream: stream,
			ctx:          ctx,
			desc:         desc,
			cc:           cc,
			method:       method,
			streamer:     streamer,
			opts:         opts,
		}, nil
	}
}

func main() {
	flag.Parse()
	// Set up a connection to the server.
	conn, err := grpc.NewClient(*addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(StreamClientInterceptor()),
	)

	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewFaultTolerantClient(conn)

	// Contact the server and print out its response.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch *mode {
	case "stream":
		stream, err := c.Stream(ctx, &pb.Request{Id: "1", Payload: "mr kim"})
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("stream error: %v", err)
				break
			}
			log.Printf("response: %v", resp.GetResult())
		}
	default:
		resp, err := c.Execute(ctx, &pb.Request{Id: "1", Payload: "mr kim"})
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		log.Printf("response: %v", resp.GetResult())
	}

}
