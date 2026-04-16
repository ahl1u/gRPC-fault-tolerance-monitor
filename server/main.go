package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	pb "github.com/ahl1u/gRPC-fault-tolerance-monitor/proto"
)

var (
	port = flag.Int("port", 50051, "The server port")
)

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
	s := grpc.NewServer()
	pb.RegisterFaultTolerantServer(s, &server{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}