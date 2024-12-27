package main

import (
	"context"
	"log"
	"net"

	dexv1 "github.com/ekinolik/jax/api/proto/dex/v1"
	marketv1 "github.com/ekinolik/jax/api/proto/market/v1"
	"github.com/ekinolik/jax/internal/config"
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
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	lis, err := net.Listen("tcp", cfg.GRPCHost)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	dexService := service.NewDexService(cfg)
	marketService := service.NewMarketService(cfg)

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(connectionInterceptor),
	)
	dexv1.RegisterDexServiceServer(grpcServer, dexService)
	marketv1.RegisterMarketServiceServer(grpcServer, marketService)
	reflection.Register(grpcServer)

	log.Printf("Starting gRPC server on %s", cfg.GRPCHost)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
