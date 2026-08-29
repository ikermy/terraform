package main

import (
	"log"
	"net"

	"google.golang.org/grpc"

	mockgrpc "terraform-provider-ai/api/grpc"
	aiv1 "terraform-provider-ai/proto/ai/v1"
)

func main() {
	lis, err := net.Listen("tcp", ":9090")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	mock := mockgrpc.NewServer()
	aiv1.RegisterClusterServiceServer(s, mock)
	aiv1.RegisterJobServiceServer(s, mock)

	log.Println("Mock gRPC API running on :9090")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
