package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"
	"time"

	marketv1 "github.com/ekinolik/jax/api/proto/market/v1"
	optionv1 "github.com/ekinolik/jax/api/proto/option/v1"
	"github.com/ekinolik/jax/internal/config"
	"github.com/ekinolik/jax/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

func init() {
	// Configure logging
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	log.SetOutput(os.Stdout)
}

// loadTLSCredentials loads TLS credentials for mutual TLS
func loadTLSCredentials() (credentials.TransportCredentials, error) {
	// Load certificate of the CA who signed client's certificate
	pemClientCA, err := ioutil.ReadFile("certs/ca/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("failed to read client CA certificate: %v", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemClientCA) {
		return nil, fmt.Errorf("failed to add client CA's certificate")
	}

	// Load server's certificate and private key
	serverCert, err := tls.LoadX509KeyPair("certs/server/server.crt", "certs/server/server.key")
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate and key: %v", err)
	}

	// Create the credentials and return it
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
	}

	return credentials.NewTLS(config), nil
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

// recoveryInterceptor recovers from panics and converts them to gRPC errors
func recoveryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			// Get stack trace
			buf := make([]byte, 64<<10)
			n := runtime.Stack(buf, false)
			stackTrace := string(buf[:n])

			log.Printf("[PANIC] method=%s request=%+v\npanic=%v\n%s",
				info.FullMethod,
				req,
				r,
				stackTrace)
			err = status.Errorf(codes.Internal, "Internal server error")
		}
	}()
	return handler(ctx, req)
}

// chainInterceptors creates a single interceptor from multiple interceptors
func chainInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			chain = func(currentInter grpc.UnaryServerInterceptor, currentHandler grpc.UnaryHandler) grpc.UnaryHandler {
				return func(currentCtx context.Context, currentReq interface{}) (interface{}, error) {
					return currentInter(currentCtx, currentReq, info, currentHandler)
				}
			}(interceptors[i], chain)
		}
		return chain(ctx, req)
	}
}

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Load TLS credentials
	tlsCredentials, err := loadTLSCredentials()
	if err != nil {
		log.Fatalf("failed to load TLS credentials: %v", err)
	}

	// Create gRPC server with TLS and interceptors
	s := grpc.NewServer(
		grpc.Creds(tlsCredentials),
		grpc.UnaryInterceptor(chainInterceptors(recoveryInterceptor, loggingInterceptor)),
	)

	// Register services
	optionService, err := service.NewOptionService(cfg)
	if err != nil {
		log.Fatalf("failed to create option service: %v", err)
	}
	optionv1.RegisterOptionServiceServer(s, optionService)

	marketService, err := service.NewMarketService(cfg)
	if err != nil {
		log.Fatalf("failed to create market service: %v", err)
	}
	marketv1.RegisterMarketServiceServer(s, marketService)

	// Register reflection service on gRPC server
	reflection.Register(s)

	log.Printf("Starting gRPC server on port %d with mTLS enabled", cfg.Port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
