package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	marketv1 "github.com/ekinolik/jax/api/proto/market/v1"
	optionv1 "github.com/ekinolik/jax/api/proto/option/v1"
	"github.com/ekinolik/jax/internal/cache"
	"github.com/ekinolik/jax/internal/config"
	intconfluence "github.com/ekinolik/jax/internal/confluence"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/ekinolik/jax/internal/scheduler"
	"github.com/ekinolik/jax/internal/service"
	"github.com/ekinolik/jax/internal/stream"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	log.SetOutput(os.Stdout)
}

func loadTLSCredentials() (credentials.TransportCredentials, error) {
	pemClientCA, err := ioutil.ReadFile("certs/ca/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("failed to read client CA certificate: %v", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemClientCA) {
		return nil, fmt.Errorf("failed to add client CA's certificate")
	}

	serverCert, err := tls.LoadX509KeyPair("certs/server/server.crt", "certs/server/server.key")
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate and key: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
	}

	return credentials.NewTLS(tlsConfig), nil
}

func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	clientIP := "unknown"
	if p, ok := peer.FromContext(ctx); ok {
		clientIP = p.Addr.String()
	}

	log.Printf("[gRPC] %s - client_ip=%s method=%s request=%+v",
		"REQUEST", clientIP, info.FullMethod, req)

	resp, err := handler(ctx, req)

	duration := time.Since(start)

	if err != nil {
		st, _ := status.FromError(err)
		log.Printf("[gRPC] %s - client_ip=%s method=%s code=%s message=%q duration=%s",
			"ERROR", clientIP, info.FullMethod, st.Code(), st.Message(), duration)
		return nil, err
	}

	log.Printf("[gRPC] %s - client_ip=%s method=%s duration=%s",
		"SUCCESS", clientIP, info.FullMethod, duration)

	return resp, nil
}

func recoveryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
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

func startCacheAndCreateScheduler(cfg *config.Config) *scheduler.Scheduler {
	cacheConfig := cache.Config{
		StorageType: cache.Disk,
		BasePath:    cfg.CacheDir,
		MaxSize:     cfg.DiskCacheLimit,
		TypeConfigs: map[cache.DataType]cache.TypeConfig{
			cache.OptionChains: {
				StorageType: cache.Disk,
				TTL:         cfg.DexCacheTTL,
				Compression: true,
				KeyPrefix:   "options",
			},
			cache.LastTrades: {
				StorageType: cache.Memory,
				TTL:         cfg.MarketCacheTTL,
				Compression: false,
				KeyPrefix:   "last-trade",
			},
			cache.Aggregates: {
				StorageType: cache.Disk,
				TTL:         cfg.AggregateCacheTTL,
				Compression: true,
				KeyPrefix:   "aggs",
			},
		},
	}
	cacheManager, err := cache.NewManager(cacheConfig)
	if err != nil {
		log.Fatalf("Failed to create cache manager: %v", err)
	}

	client := polygon.NewClient(cfg)
	sched := scheduler.NewScheduler(cacheManager, client)

	if err := sched.LoadTasks("cache-configs/cache_tasks.yaml", time.Minute); err != nil {
		log.Fatalf("Failed to load tasks: %v", err)
	}

	return sched
}

func startConfluence(cfg *config.Config) (*intconfluence.Processor, *stream.Hub, *http.Server) {
	settingsPath := filepath.Join("confluence-configs", "settings.yaml")
	sectorsPath := filepath.Join("confluence-configs", "sic_sectors.yaml")

	settings, err := pkgconfluence.LoadSettings(settingsPath)
	if err != nil {
		log.Fatalf("Failed to load confluence settings: %v", err)
	}
	sectors, err := pkgconfluence.LoadSICSectors(sectorsPath)
	if err != nil {
		log.Fatalf("Failed to load sic sectors: %v", err)
	}

	oiCache, err := intconfluence.NewOICache("")
	if err != nil {
		log.Fatalf("Failed to create OI cache: %v", err)
	}

	hub, err := stream.NewHub(cfg.PolygonAPIKey)
	if err != nil {
		log.Fatalf("Failed to create stream hub: %v", err)
	}

	client := polygon.NewClient(cfg)
	registry := intconfluence.NewRegistry(settings.MaxActiveTickers)
	processor := intconfluence.NewProcessor(settings, sectors, registry, oiCache, client, hub)

	ctx := context.Background()
	if err := hub.Start(ctx); err != nil {
		log.Fatalf("Failed to start stream hub: %v", err)
	}
	if err := processor.Start(ctx); err != nil {
		log.Fatalf("Failed to start confluence processor: %v", err)
	}
	log.Printf("[confluence] processor started (max_active_tickers=%d)", settings.MaxActiveTickers)

	var debugSrv *http.Server
	if addr := intconfluence.DebugPortFromEnv(os.Getenv("JAX_CONFLUENCE_DEBUG_PORT")); addr != "" {
		debugSrv = intconfluence.StartDebugServer(addr, processor)
	}

	return processor, hub, debugSrv
}

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	sched := startCacheAndCreateScheduler(cfg)
	sched.Start()
	defer sched.Stop()

	processor, hub, debugSrv := startConfluence(cfg)
	defer hub.Stop()
	defer processor.Stop()
	if debugSrv != nil {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = debugSrv.Shutdown(shutdownCtx)
		}()
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	tlsCredentials, err := loadTLSCredentials()
	if err != nil {
		log.Fatalf("failed to load TLS credentials: %v", err)
	}

	s := grpc.NewServer(
		grpc.Creds(tlsCredentials),
		grpc.UnaryInterceptor(chainInterceptors(recoveryInterceptor, loggingInterceptor)),
	)

	optionService := service.NewOptionService(cfg, sched.GetCache())
	optionv1.RegisterOptionServiceServer(s, optionService)

	marketService := service.NewMarketService(cfg, sched.GetCache())
	marketv1.RegisterMarketServiceServer(s, marketService)

	reflection.Register(s)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Starting gRPC server on port %d with mTLS enabled", cfg.Port)
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	<-sigCh
	log.Printf("Shutting down...")
	s.GracefulStop()
}
