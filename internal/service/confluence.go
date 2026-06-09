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

// defaultGetConfluenceTimeout bounds subscribe + live-field refresh only; OI/bootstrap runs in background.
const defaultGetConfluenceTimeout = 15 * time.Second

// defaultOIWaitBudget is how long a unary handler may wait when background OI/bootstrap is in flight.
const defaultOIWaitBudget = 90 * time.Second

// ConfluenceProcessor is the subset of the confluence processor used by gRPC handlers.
type ConfluenceProcessor interface {
	GetSnapshot(ticker string) (*pkgconfluence.ConfluenceSnapshot, bool)
	Watch(ctx context.Context, ticker string) (<-chan *pkgconfluence.ConfluenceSnapshot, func(), error)
	OnSubscribe(ctx context.Context, ticker string, now time.Time) error
	OnUnsubscribe(ticker string)
	StopBackgroundWarmup(ticker string)
	BootstrapInProgress(ticker string) bool
	RecomputeIfReady(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, bool)
	BootstrapSnapshot(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, error)
	RefreshSnapshotLive(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, error)
}

// ConfluenceService serves ConfluenceService gRPC requests.
type ConfluenceService struct {
	confluencev1.UnimplementedConfluenceServiceServer
	processor    ConfluenceProcessor
	timeout      time.Duration
	oiWaitBudget time.Duration
}

// NewConfluenceService creates a ConfluenceService backed by the given processor.
func NewConfluenceService(processor ConfluenceProcessor) *ConfluenceService {
	return &ConfluenceService{
		processor:    processor,
		timeout:      defaultGetConfluenceTimeout,
		oiWaitBudget: defaultOIWaitBudget,
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
		return s.refreshLiveSnapshot(ctx, ticker, method, snap)
	}

	handlerCtx, handlerCancel := context.WithTimeout(ctx, s.timeout)
	defer handlerCancel()

	if err := s.processor.OnSubscribe(handlerCtx, ticker, time.Now().UTC()); err != nil {
		return nil, mapConfluenceError(err)
	}
	defer s.processor.OnUnsubscribe(ticker)

	if snap, ok := s.processor.GetSnapshot(ticker); ok && snap != nil && !intconfluence.SnapshotNeedsBootstrap(snap) {
		return s.refreshLiveSnapshot(handlerCtx, ticker, method, snap)
	}

	snap, ok := s.processor.GetSnapshot(ticker)
	if !ok || snap == nil {
		return nil, status.Errorf(codes.Unavailable, "no snapshot for %s after subscribe", ticker)
	}

	// Hybrid wait: return immediately when bootstrap is idle; poll up to oiWaitBudget while background OI runs.
	if intconfluence.SnapshotNeedsBootstrap(snap) && s.processor.BootstrapInProgress(ticker) {
		waitCtx, waitCancel := context.WithTimeout(ctx, s.oiWaitBudget)
		defer waitCancel()
		if ready := s.waitForScoredSnapshot(waitCtx, ticker); ready != nil {
			snap = ready
		}
	} else if intconfluence.SnapshotNeedsBootstrap(snap) {
		if ready, ok := s.processor.RecomputeIfReady(handlerCtx, ticker); ok {
			snap = ready
		}
	}

	return s.refreshLiveSnapshot(handlerCtx, ticker, method, snap)
}

func (s *ConfluenceService) waitForScoredSnapshot(ctx context.Context, ticker string) *pkgconfluence.ConfluenceSnapshot {
	const pollInterval = 100 * time.Millisecond
	for {
		if ready, ok := s.processor.RecomputeIfReady(ctx, ticker); ok {
			return ready
		}
		if snap, ok := s.processor.GetSnapshot(ticker); ok && snap != nil && !intconfluence.SnapshotNeedsBootstrap(snap) {
			return snap
		}
		select {
		case <-ctx.Done():
			if snap, ok := s.processor.GetSnapshot(ticker); ok {
				return snap
			}
			return nil
		case <-time.After(pollInterval):
		}
	}
}

func (s *ConfluenceService) refreshLiveSnapshot(
	ctx context.Context,
	ticker, method string,
	fallback *pkgconfluence.ConfluenceSnapshot,
) (*pkgconfluence.ConfluenceSnapshot, error) {
	refreshed, err := s.processor.RefreshSnapshotLive(ctx, ticker)
	if err != nil {
		log.Printf("[confluence] %s live refresh %s: %v", method, ticker, err)
		return fallback, nil
	}
	if refreshed != nil {
		return refreshed, nil
	}
	return fallback, nil
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
