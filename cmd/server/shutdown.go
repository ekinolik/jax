package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	intconfluence "github.com/ekinolik/jax/internal/confluence"
	"github.com/ekinolik/jax/internal/scheduler"
	"github.com/ekinolik/jax/internal/stream"
	"google.golang.org/grpc"
)

const (
	processorStopTimeout = 10 * time.Second
	hubStopTimeout       = 10 * time.Second
	grpcGracefulTimeout  = 15 * time.Second
	schedulerStopTimeout = 10 * time.Second
	debugShutdownTimeout = 5 * time.Second
)

// shutdownSequence documents the intentional server teardown order.
func shutdownSequence() []string {
	return []string{
		"processor",
		"stream-hub",
		"grpc",
		"scheduler",
		"debug-http",
	}
}

type shutdownTargets struct {
	grpcServer *grpc.Server
	listener   net.Listener
	processor  *intconfluence.Processor
	hub        *stream.Hub
	scheduler  *scheduler.Scheduler
	debugSrv   *http.Server
}

func shutdownServer(targets shutdownTargets, force <-chan os.Signal) {
	log.Printf("Shutting down (order: %v)...", shutdownSequence())

	// Stop background work first so long-running gRPC handlers can unwind.
	stopWithTimeout("processor", targets.processor.Stop, processorStopTimeout)
	stopWithTimeout("stream hub", targets.hub.Stop, hubStopTimeout)

	if targets.listener != nil {
		_ = targets.listener.Close()
	}

	grpcDone := make(chan struct{})
	go func() {
		targets.grpcServer.GracefulStop()
		close(grpcDone)
	}()

	select {
	case <-grpcDone:
	case <-time.After(grpcGracefulTimeout):
		log.Printf("GracefulStop timed out after %s; forcing Stop", grpcGracefulTimeout)
		targets.grpcServer.Stop()
	case <-force:
		log.Printf("Second shutdown signal received; forcing Stop")
		targets.grpcServer.Stop()
	}

	stopWithTimeout("scheduler", targets.scheduler.Stop, schedulerStopTimeout)

	if targets.debugSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), debugShutdownTimeout)
		defer cancel()
		if err := targets.debugSrv.Shutdown(ctx); err != nil {
			log.Printf("debug HTTP shutdown: %v", err)
		}
	}
}

func stopWithTimeout(name string, stop func(), timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		log.Printf("WARNING: %s stop timed out after %s; continuing shutdown", name, timeout)
	}
}
