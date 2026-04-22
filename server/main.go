package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/metadata"

	pb "github.com/ahl1u/gRPC-fault-tolerance-monitor/proto"
	"google.golang.org/grpc"
)

var (
	port = flag.Int("port", 50051, "The server port")
	isLeader = flag.Bool("leader", false, "whether this server is leader currently")
	leaderAddr = flag.String("leader-addr", "localhost:50051", "current leader address")
)

type replyCache struct {
	// Data structure to hold cached responses
	mu      sync.Mutex              // lock to prevent race conditions when multiple client requests arrive concurrently
	entries map[string]*pb.Response // maps a string request ID to the response the server produced
}

func serverInterceptor(cache *replyCache) grpc.UnaryServerInterceptor {
	// Returns a function that gRPC will call on every incoming request
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		r, ok := req.(*pb.Request)

		if !ok {
			return handler(ctx, req)
		}

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
	mu       sync.Mutex
	isLeader bool
	leaderAddr string
}

func (s *server) Promote(ctx context.Context, req *pb.PromoteRequest) (*pb.PromoteResponse, error) {
	s.mu.Lock()
	s.isLeader = true
	s.mu.Unlock()
	return &pb.PromoteResponse{}, nil
}

func (s *server) UpdateLeader(ctx context.Context, req *pb.UpdateLeaderRequest) (*pb.UpdateLeaderResponse, error) {
    s.mu.Lock()
    s.leaderAddr = req.NewLeaderAddr
    s.mu.Unlock()
    return &pb.UpdateLeaderResponse{}, nil
}


func (s *server) Execute(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	s.mu.Lock()
	isLeader := s.isLeader
	leaderAddr := s.leaderAddr
	s.mu.Unlock()

	if (!isLeader) {
		header := metadata.Pairs("leader-addr", leaderAddr)
		grpc.SendHeader(ctx, header)
		return nil, status.Error(codes.Unavailable, "not the leader")
	}
	log.Printf("received: %v", req.Payload)
	return &pb.Response{Id: req.Id, Result: "ok"}, nil
}

func (s *server) Stream(req *pb.Request, stream pb.FaultTolerant_StreamServer) error {
	for i := 0; i < 10; i++ {
		if err := stream.Send(&pb.Response{Id: req.Id, Result: fmt.Sprintf("message %d", i)}); err != nil {
			return err
		}
		time.Sleep(500*time.Millisecond)
	}
	return nil
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	cache := &replyCache{entries: make(map[string]*pb.Response)}
	s := grpc.NewServer(grpc.UnaryInterceptor(serverInterceptor(cache)))
	pb.RegisterFaultTolerantServer(s, &server{isLeader : *isLeader, leaderAddr: *leaderAddr,})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
