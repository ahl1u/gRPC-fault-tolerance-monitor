package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"

	pb "github.com/ahl1u/gRPC-fault-tolerance-monitor/proto"
	"google.golang.org/grpc"
)

var (
	port = flag.Int("port", 50051, "The server port")
)

type replyCache struct {
	// Data structure to hold cached responses
	mu      sync.Mutex              // lock to prevent race conditions when multiple client requests arrive concurrently
	entries map[string]*pb.Response // maps a string request ID to the response the server produced
}

func serverInterceptor(cache *replyCache) grpc.UnaryServerInterceptor {
	// Returns a function that gRPC will call on every incoming request
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		r := req.(*pb.Request)

		// Lock the cache, look up request ID.
		// If found, return cached response and exit.
		cache.mu.Lock()
		if cached, ok := cache.entries[r.Id]; ok {
			cache.mu.Unlock()
			log.Printf("cache hit for id=%s, skipping execution", r.Id)
			return cached, nil
		}
		// If not found, unlock and continue
		cache.mu.Unlock()

		resp, err := handler(ctx, req)
		if err != nil {
			return nil, err
		}

		cache.mu.Lock()
		cache.entries[r.Id] = resp.(*pb.Response)
		cache.mu.Unlock()

		return resp, nil
	}
}

type server struct {
	pb.UnimplementedFaultTolerantServer
}

func (s *server) Execute(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	log.Printf("received: %v", req.Payload)
	return &pb.Response{Id: req.Id, Result: "ok"}, nil
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	cache := &replyCache{entries: make(map[string]*pb.Response)}
	s := grpc.NewServer(grpc.UnaryInterceptor(serverInterceptor(cache)))
	pb.RegisterFaultTolerantServer(s, &server{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
