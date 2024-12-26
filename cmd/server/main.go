package main

import (
	"context"
	"log"
	"net"
	"os"

	dexv1 "github.com/ekinolik/jax/api/proto/dex/v1"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/ekinolik/jax/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
)

// connectionInterceptor logs new connections
func connectionInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if p, ok := peer.FromContext(ctx); ok {
		log.Printf("[CONNECTION] New connection from %s", p.Addr.String())
	}
	return handler(ctx, req)
}

func main() {
	apiKey := os.Getenv("POLYGON_API_KEY")
	if apiKey == "" {
		log.Fatal("POLYGON_API_KEY environment variable is required")
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	polygonClient := polygon.NewClient(apiKey)
	dexService := service.NewDexService(polygonClient)

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(connectionInterceptor),
	)
	dexv1.RegisterDexServiceServer(grpcServer, dexService)
	reflection.Register(grpcServer)

	log.Printf("Starting gRPC server on :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
