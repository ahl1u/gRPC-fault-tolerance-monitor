package main

import (
	"context"
	"flag"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "github.com/ahl1u/gRPC-fault-tolerance-monitor/proto"
)

var (
	addr = flag.String("addr", "localhost:50051", "the address to connect to")
)

func main() {
	flag.Parse()
	// Set up a connection to the server.
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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