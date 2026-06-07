package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	confluencev1 "github.com/ekinolik/jax/api/proto/confluence/v1"
	intconfluence "github.com/ekinolik/jax/internal/confluence"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultGetConfluenceTimeout = 90 * time.Second

// ConfluenceProcessor is the subset of the confluence processor used by gRPC handlers.
type ConfluenceProcessor interface {
	GetSnapshot(ticker string) (*pkgconfluence.ConfluenceSnapshot, bool)
	Watch(ctx context.Context, ticker string) (<-chan *pkgconfluence.ConfluenceSnapshot, func(), error)
	OnSubscribe(ctx context.Context, ticker string, now time.Time) error
	OnUnsubscribe(ticker string)
	BootstrapSnapshot(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, error)
}

// ConfluenceService serves ConfluenceService gRPC requests.
type ConfluenceService struct {
	confluencev1.UnimplementedConfluenceServiceServer
	processor ConfluenceProcessor
	timeout   time.Duration
}

// NewConfluenceService creates a ConfluenceService backed by the given processor.
func NewConfluenceService(processor ConfluenceProcessor) *ConfluenceService {
	return &ConfluenceService{
		processor: processor,
		timeout:   defaultGetConfluenceTimeout,
	}
}

func parseConfluenceTicker(method, raw string) (string, error) {
	ticker := pkgconfluence.NormalizeTicker(raw)
	LogRequest(method, map[string]interface{}{"ticker": ticker})
	if err := pkgconfluence.ValidateTicker(ticker); err != nil {
		return "", status.Errorf(codes.InvalidArgument, "invalid ticker: %v", err)
	}
	return ticker, nil
}

// GetConfluenceSummary returns a human-readable summary projected from the latest snapshot.
func (s *ConfluenceService) GetConfluenceSummary(ctx context.Context, req *confluencev1.GetConfluenceRequest) (*confluencev1.ConfluenceSummary, error) {
	snap, err := s.fetchSnapshot(ctx, req.GetTicker(), "GetConfluenceSummary")
	if err != nil {
		return nil, err
	}
	return SummaryToProto(pkgconfluence.SummaryFromSnapshot(*snap)), nil
}

// GetConfluence returns the latest snapshot, bootstrapping from Massive when no scored cache exists.
func (s *ConfluenceService) GetConfluence(ctx context.Context, req *confluencev1.GetConfluenceRequest) (*confluencev1.ConfluenceSnapshot, error) {
	snap, err := s.fetchSnapshot(ctx, req.GetTicker(), "GetConfluence")
	if err != nil {
		return nil, err
	}
	return SnapshotToProto(snap), nil
}

func (s *ConfluenceService) fetchSnapshot(ctx context.Context, rawTicker, method string) (*pkgconfluence.ConfluenceSnapshot, error) {
	ticker, err := parseConfluenceTicker(method, rawTicker)
	if err != nil {
		return nil, err
	}

	if snap, ok := s.processor.GetSnapshot(ticker); ok && snap != nil && !intconfluence.SnapshotNeedsBootstrap(snap) {
		return snap, nil
	}

	handlerCtx, handlerCancel := context.WithTimeout(ctx, s.timeout)
	defer handlerCancel()

	if err := s.processor.OnSubscribe(handlerCtx, ticker, time.Now().UTC()); err != nil {
		return nil, mapConfluenceError(err)
	}
	defer s.processor.OnUnsubscribe(ticker)

	if snap, ok := s.processor.GetSnapshot(ticker); ok && snap != nil && !intconfluence.SnapshotNeedsBootstrap(snap) {
		return snap, nil
	}

	if snap, err := s.processor.BootstrapSnapshot(handlerCtx, ticker); err == nil && snap != nil && !intconfluence.SnapshotNeedsBootstrap(snap) {
		return snap, nil
	} else if err != nil {
		log.Printf("[confluence] %s bootstrap %s: %v", method, ticker, err)
	}

	return waitForReadySnapshot(handlerCtx, s.processor, ticker, s.timeout)
}

// WatchConfluence streams snapshot updates when score or signal status changes.
func (s *ConfluenceService) WatchConfluence(req *confluencev1.WatchConfluenceRequest, stream confluencev1.ConfluenceService_WatchConfluenceServer) error {
	ticker, err := parseConfluenceTicker("WatchConfluence", req.GetTicker())
	if err != nil {
		return err
	}

	ctx := stream.Context()
	ch, unsub, err := s.processor.Watch(ctx, ticker)
	if err != nil {
		return mapConfluenceError(err)
	}
	defer unsub()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case snap, ok := <-ch:
			if !ok {
				return nil
			}
			if snap == nil {
				continue
			}
			if err := stream.Send(SnapshotToProto(snap)); err != nil {
				return err
			}
		}
	}
}

func waitForReadySnapshot(ctx context.Context, processor ConfluenceProcessor, ticker string, timeout time.Duration) (*pkgconfluence.ConfluenceSnapshot, error) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	tickerWait := time.NewTicker(100 * time.Millisecond)
	defer tickerWait.Stop()

	for {
		if snap, ok := processor.GetSnapshot(ticker); ok && snap != nil && !intconfluence.SnapshotNeedsBootstrap(snap) {
			return snap, nil
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, status.Errorf(
					codes.DeadlineExceeded,
					"no scored confluence snapshot for %s within %s; bootstrap may have failed or data is unavailable",
					ticker,
					timeout,
				)
			}
			return nil, status.Errorf(codes.Canceled, "request canceled while waiting for %s snapshot", ticker)
		case <-tickerWait.C:
		}
	}
}

func mapConfluenceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return status.Errorf(codes.Canceled, "%v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Errorf(codes.DeadlineExceeded, "%v", err)
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "max active tickers"):
		return status.Errorf(codes.ResourceExhausted, "%s", msg)
	case strings.Contains(msg, "invalid ticker"), strings.Contains(msg, "empty ticker"):
		return status.Errorf(codes.InvalidArgument, "%s", msg)
	default:
		log.Printf("[confluence] internal error: %v", err)
		return status.Error(codes.Internal, "confluence service unavailable")
	}
}
