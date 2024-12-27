package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	dexv1 "github.com/ekinolik/jax/api/proto/dex/v1"
	marketv1 "github.com/ekinolik/jax/api/proto/market/v1"
	"github.com/ekinolik/jax/internal/config"
	"github.com/ekinolik/jax/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

func init() {
	// Configure logging
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	log.SetOutput(os.Stdout)
}

// loggingInterceptor logs all gRPC method calls
func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	clientIP := "unknown"
	if p, ok := peer.FromContext(ctx); ok {
		clientIP = p.Addr.String()
	}

	// Log the request
	log.Printf("[gRPC] %s - client_ip=%s method=%s request=%+v",
		"REQUEST", clientIP, info.FullMethod, req)

	// Call the handler
	resp, err := handler(ctx, req)

	// Calculate duration
	duration := time.Since(start)

	if err != nil {
		st, _ := status.FromError(err)
		log.Printf("[gRPC] %s - client_ip=%s method=%s code=%s message=%q duration=%s",
			"ERROR", clientIP, info.FullMethod, st.Code(), st.Message(), duration)
		return nil, err
	}

	// Log the response
	log.Printf("[gRPC] %s - client_ip=%s method=%s duration=%s",
		"SUCCESS", clientIP, info.FullMethod, duration)

	return resp, nil
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
		grpc.UnaryInterceptor(loggingInterceptor),
	)
	dexv1.RegisterDexServiceServer(grpcServer, dexService)
	marketv1.RegisterMarketServiceServer(grpcServer, marketService)
	reflection.Register(grpcServer)

	log.Printf("Starting gRPC server on %s", cfg.GRPCHost)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
