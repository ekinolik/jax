package service

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	confluencev1 "github.com/ekinolik/jax/api/proto/confluence/v1"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type mockConfluenceProcessor struct {
	snapshots        map[string]*pkgconfluence.ConfluenceSnapshot
	onSubErr         error
	watchErr         error
	bootstrapErr     error
	subscribeCount   int
	unsubscribeCount int
}

func (m *mockConfluenceProcessor) GetSnapshot(ticker string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	snap, ok := m.snapshots[ticker]
	return snap, ok
}

func (m *mockConfluenceProcessor) OnSubscribe(ctx context.Context, ticker string, now time.Time) error {
	m.subscribeCount++
	if m.onSubErr != nil {
		return m.onSubErr
	}
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if _, ok := m.snapshots[ticker]; !ok {
		m.snapshots[ticker] = &pkgconfluence.ConfluenceSnapshot{
			Ticker:    ticker,
			OIStatus:  pkgconfluence.OIStatusLoading,
			UpdatedAt: now,
		}
	}
	return nil
}

func (m *mockConfluenceProcessor) OnUnsubscribe(ticker string) {
	m.unsubscribeCount++
}

func (m *mockConfluenceProcessor) StopBackgroundWarmup(ticker string) {}

func (m *mockConfluenceProcessor) BootstrapInProgress(ticker string) bool {
	return false
}

func (m *mockConfluenceProcessor) RecomputeIfReady(context.Context, string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	return nil, false
}

func (m *mockConfluenceProcessor) BootstrapSnapshot(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, error) {
	if m.bootstrapErr != nil {
		return nil, m.bootstrapErr
	}
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if snap, ok := m.snapshots[ticker]; ok && snap != nil && snap.OIStatus == pkgconfluence.OIStatusReady && snap.Levels.GammaFlip > 0 {
		return snap, nil
	}
	return nil, context.DeadlineExceeded
}

func (m *mockConfluenceProcessor) Watch(ctx context.Context, ticker string) (<-chan *pkgconfluence.ConfluenceSnapshot, func(), error) {
	if m.watchErr != nil {
		return nil, nil, m.watchErr
	}
	ch := make(chan *pkgconfluence.ConfluenceSnapshot, 1)
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if snap, ok := m.snapshots[ticker]; ok {
		ch <- snap
	}
	close(ch)
	return ch, func() {}, nil
}

func (m *mockConfluenceProcessor) RefreshSnapshotLive(ctx context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, error) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if snap, ok := m.snapshots[ticker]; ok {
		return snap, nil
	}
	return nil, nil
}

type watchStreamMock struct {
	ctx    context.Context
	sent   []*confluencev1.ConfluenceSnapshot
	sendErr error
}

func (s *watchStreamMock) SetHeader(metadata.MD) error  { return nil }
func (s *watchStreamMock) SendHeader(metadata.MD) error { return nil }
func (s *watchStreamMock) SetTrailer(metadata.MD)       {}
func (s *watchStreamMock) Context() context.Context     { return s.ctx }
func (s *watchStreamMock) SendMsg(m any) error          { return nil }
func (s *watchStreamMock) RecvMsg(m any) error          { return nil }

func (s *watchStreamMock) Send(snap *confluencev1.ConfluenceSnapshot) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent = append(s.sent, snap)
	return nil
}

func TestGetConfluenceSummary_cachedSnapshot(t *testing.T) {
	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:          "NVDA",
		Spot:            120.5,
		Score:           62,
		ReadinessBand:   pkgconfluence.ReadinessPossibleEntry,
		SellScore:       28,
		SellReadiness:   pkgconfluence.SellHold,
		ExitAction:      pkgconfluence.ExitHold,
		OIStatus:        pkgconfluence.OIStatusReady,
		MarketStatus:    pkgconfluence.MarketStatusOpen,
		UpdatedAt:       time.Now().UTC(),
		GammaRegime:     pkgconfluence.GammaPositive,
		DistanceToEntry: pkgconfluence.EntryEarly,
		Levels: pkgconfluence.Levels{
			GammaFlip:            118.0,
			NearestSupport:       118.5,
			NearestResistance:    125.0,
			HasNearestSupport:    true,
			HasNearestResistance: true,
		},
		BuySignals: []pkgconfluence.Signal{
			{Name: "gamma_support", Status: pkgconfluence.SignalAligned},
		},
	}
	proc := &mockConfluenceProcessor{snapshots: map[string]*pkgconfluence.ConfluenceSnapshot{"NVDA": snap}}
	svc := NewConfluenceService(proc)

	resp, err := svc.GetConfluenceSummary(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "nvda"})
	if err != nil {
		t.Fatalf("GetConfluenceSummary: %v", err)
	}
	if resp.Ticker != "NVDA" {
		t.Errorf("ticker: got %q", resp.Ticker)
	}
	if resp.Verdict == nil || resp.Verdict.Buy == nil {
		t.Fatal("expected buy verdict")
	}
	if resp.Verdict.Buy.Score != 62 || resp.Verdict.Buy.Label != "Watch for entry" {
		t.Errorf("buy verdict: %+v", resp.Verdict.Buy)
	}
	if resp.TradeSetup == nil || resp.TradeSetup.Archetype != "mean_reversion" {
		t.Errorf("trade_setup: %+v", resp.TradeSetup)
	}
}

func TestGetConfluence_cachedSnapshot(t *testing.T) {
	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:        "NVDA",
		Score:         55,
		ReadinessBand: pkgconfluence.ReadinessPossibleEntry,
		OIStatus:      pkgconfluence.OIStatusReady,
		UpdatedAt:     time.Now().UTC(),
		Levels:        pkgconfluence.Levels{GammaFlip: 118.0},
	}
	proc := &mockConfluenceProcessor{snapshots: map[string]*pkgconfluence.ConfluenceSnapshot{"NVDA": snap}}
	svc := NewConfluenceService(proc)

	resp, err := svc.GetConfluence(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "nvda"})
	if err != nil {
		t.Fatalf("GetConfluence: %v", err)
	}
	if resp.Ticker != "NVDA" || resp.ConfluenceScore != 55 {
		t.Errorf("response: %+v", resp)
	}
}

func TestGetConfluence_invalidTicker(t *testing.T) {
	svc := NewConfluenceService(&mockConfluenceProcessor{})
	_, err := svc.GetConfluence(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestGetConfluence_coldStartReturnsWithinTimeout(t *testing.T) {
	proc := &mockConfluenceProcessor{snapshots: map[string]*pkgconfluence.ConfluenceSnapshot{}}
	svc := NewConfluenceService(proc)
	svc.timeout = 200 * time.Millisecond

	start := time.Now()
	resp, err := svc.GetConfluence(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "NVDA"})
	if err != nil {
		t.Fatalf("GetConfluence: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("expected response within handler timeout, took %s", time.Since(start))
	}
	if resp.Ticker != "NVDA" {
		t.Fatalf("ticker: got %q", resp.Ticker)
	}
	if proc.subscribeCount != 1 || proc.unsubscribeCount != 1 {
		t.Errorf("subscribe balance: sub=%d unsub=%d", proc.subscribeCount, proc.unsubscribeCount)
	}
}

func TestGetConfluence_tickerBNotBlockedByTickerA(t *testing.T) {
	blockSPY := make(chan struct{})
	proc := &blockingSubscribeProcessor{
		mockConfluenceProcessor: mockConfluenceProcessor{
			snapshots: map[string]*pkgconfluence.ConfluenceSnapshot{},
		},
		blockTicker: "SPY",
		blockCh:     blockSPY,
	}
	svc := NewConfluenceService(proc)
	svc.timeout = 2 * time.Second

	go func() {
		_, _ = svc.GetConfluence(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "SPY"})
		close(blockSPY)
	}()

	deadline := time.After(2 * time.Second)
	for proc.spySubscribeCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("SPY subscribe did not start")
		case <-time.After(10 * time.Millisecond):
		}
	}

	start := time.Now()
	resp, err := svc.GetConfluence(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "NVDA"})
	if err != nil {
		t.Fatalf("GetConfluence NVDA: %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("NVDA query blocked for %s while SPY subscribe was in flight", time.Since(start))
	}
	if resp.Ticker != "NVDA" || resp.OiStatus != string(pkgconfluence.OIStatusLoading) {
		t.Fatalf("NVDA response: %+v", resp)
	}
}

type blockingSubscribeProcessor struct {
	mockConfluenceProcessor
	blockTicker string
	blockCh     chan struct{}
	spySubs     int
	mu          sync.Mutex
}

func (b *blockingSubscribeProcessor) spySubscribeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spySubs
}

func (b *blockingSubscribeProcessor) OnSubscribe(ctx context.Context, ticker string, now time.Time) error {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if ticker == b.blockTicker {
		b.mu.Lock()
		b.spySubs++
		b.mu.Unlock()
		select {
		case <-b.blockCh:
		case <-ctx.Done():
		}
	}
	return b.mockConfluenceProcessor.OnSubscribe(ctx, ticker, now)
}

func TestGetConfluence_coldStartReturnsLoadingSnapshot(t *testing.T) {
	proc := &mockConfluenceProcessor{
		snapshots: map[string]*pkgconfluence.ConfluenceSnapshot{
			"SPY": {
				Ticker:       "SPY",
				Spot:         500,
				OIStatus:     pkgconfluence.OIStatusLoading,
				MarketStatus: pkgconfluence.MarketStatusOpen,
				UpdatedAt:    time.Now().UTC(),
			},
		},
	}
	svc := NewConfluenceService(proc)

	start := time.Now()
	resp, err := svc.GetConfluence(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "SPY"})
	if err != nil {
		t.Fatalf("expected loading snapshot, got error: %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("cold start should return immediately, took %s", time.Since(start))
	}
	if resp.OiStatus != string(pkgconfluence.OIStatusLoading) {
		t.Fatalf("oi_status: got %q want loading", resp.OiStatus)
	}
}

func TestGetConfluence_maxActiveTickers(t *testing.T) {
	proc := &mockConfluenceProcessor{
		snapshots: map[string]*pkgconfluence.ConfluenceSnapshot{},
		onSubErr:  errors.New("max active tickers (5) reached"),
	}
	svc := NewConfluenceService(proc)

	_, err := svc.GetConfluence(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "NVDA"})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected ResourceExhausted, got %v", err)
	}
}

func TestWatchConfluence_streamsSnapshot(t *testing.T) {
	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:        "AAPL",
		Score:         40,
		ReadinessBand: pkgconfluence.ReadinessCaution,
		UpdatedAt:     time.Now().UTC(),
	}
	proc := &mockConfluenceProcessor{snapshots: map[string]*pkgconfluence.ConfluenceSnapshot{"AAPL": snap}}
	svc := NewConfluenceService(proc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := &watchStreamMock{ctx: ctx}
	if err := svc.WatchConfluence(&confluencev1.WatchConfluenceRequest{Ticker: "AAPL"}, stream); err != nil {
		t.Fatalf("WatchConfluence: %v", err)
	}
	if len(stream.sent) != 1 || stream.sent[0].Ticker != "AAPL" {
		t.Fatalf("sent: %+v", stream.sent)
	}
}

func TestWatchConfluence_invalidTicker(t *testing.T) {
	svc := NewConfluenceService(&mockConfluenceProcessor{})
	stream := &watchStreamMock{ctx: context.Background()}
	err := svc.WatchConfluence(&confluencev1.WatchConfluenceRequest{Ticker: "bad ticker"}, stream)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestWatchConfluence_contextCanceled(t *testing.T) {
	ch := make(chan *pkgconfluence.ConfluenceSnapshot)
	procWatch := &blockingMockProcessor{ch: ch}
	svc := NewConfluenceService(procWatch)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := &watchStreamMock{ctx: ctx}
	err := svc.WatchConfluence(&confluencev1.WatchConfluenceRequest{Ticker: "NVDA"}, stream)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

type blockingMockProcessor struct {
	ch <-chan *pkgconfluence.ConfluenceSnapshot
}

func (b *blockingMockProcessor) GetSnapshot(string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	return nil, false
}

func (b *blockingMockProcessor) OnSubscribe(context.Context, string, time.Time) error {
	return nil
}

func (b *blockingMockProcessor) OnUnsubscribe(string) {}

func (b *blockingMockProcessor) StopBackgroundWarmup(string) {}

func (b *blockingMockProcessor) BootstrapInProgress(string) bool { return false }

func (b *blockingMockProcessor) RecomputeIfReady(context.Context, string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	return nil, false
}

func (b *blockingMockProcessor) BootstrapSnapshot(context.Context, string) (*pkgconfluence.ConfluenceSnapshot, error) {
	return nil, context.DeadlineExceeded
}

func (b *blockingMockProcessor) Watch(ctx context.Context, ticker string) (<-chan *pkgconfluence.ConfluenceSnapshot, func(), error) {
	return b.ch, func() {}, nil
}

func (b *blockingMockProcessor) RefreshSnapshotLive(context.Context, string) (*pkgconfluence.ConfluenceSnapshot, error) {
	return nil, nil
}

func TestGetConfluence_subscribeThenSnapshot(t *testing.T) {
	proc := &delayedMockProcessor{}
	svc := NewConfluenceService(proc)

	resp, err := svc.GetConfluence(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "TSLA"})
	if err != nil {
		t.Fatalf("GetConfluence: %v", err)
	}
	if resp.Ticker != "TSLA" || resp.ConfluenceScore != 80 {
		t.Errorf("response: %+v", resp)
	}
	if proc.subscribeCount != 1 || proc.unsubscribeCount != 1 {
		t.Errorf("subscribe balance: sub=%d unsub=%d", proc.subscribeCount, proc.unsubscribeCount)
	}
}

type delayedMockProcessor struct {
	subscribed       bool
	subscribeCount   int
	unsubscribeCount int
}

func (d *delayedMockProcessor) GetSnapshot(ticker string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	if !d.subscribed {
		return nil, false
	}
	return &pkgconfluence.ConfluenceSnapshot{
		Ticker:        pkgconfluence.NormalizeTicker(ticker),
		Score:         80,
		ReadinessBand: pkgconfluence.ReadinessHighConviction,
		OIStatus:      pkgconfluence.OIStatusReady,
		UpdatedAt:     time.Now().UTC(),
		Levels:        pkgconfluence.Levels{GammaFlip: 200.0},
	}, true
}

func (d *delayedMockProcessor) OnSubscribe(context.Context, string, time.Time) error {
	d.subscribed = true
	d.subscribeCount++
	return nil
}

func (d *delayedMockProcessor) OnUnsubscribe(string) {
	d.unsubscribeCount++
}

func (d *delayedMockProcessor) StopBackgroundWarmup(string) {}

func (d *delayedMockProcessor) BootstrapInProgress(string) bool { return false }

func (d *delayedMockProcessor) RecomputeIfReady(context.Context, string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	return nil, false
}

func (d *delayedMockProcessor) BootstrapSnapshot(context.Context, string) (*pkgconfluence.ConfluenceSnapshot, error) {
	return nil, context.DeadlineExceeded
}

func (d *delayedMockProcessor) Watch(context.Context, string) (<-chan *pkgconfluence.ConfluenceSnapshot, func(), error) {
	return nil, nil, io.EOF
}

func (d *delayedMockProcessor) RefreshSnapshotLive(context.Context, string) (*pkgconfluence.ConfluenceSnapshot, error) {
	return nil, nil
}

func TestGetConfluenceSummary_secondQueryReturnsScoredAfterWarmup(t *testing.T) {
	loading := &pkgconfluence.ConfluenceSnapshot{
		Ticker:       "SNDK",
		Spot:         1667.43,
		OIStatus:     pkgconfluence.OIStatusLoading,
		MarketStatus: pkgconfluence.MarketStatusOpen,
		UpdatedAt:    time.Now().UTC(),
	}
	scored := &pkgconfluence.ConfluenceSnapshot{
		Ticker:            "SNDK",
		Spot:              1667.43,
		Score:             58,
		ReadinessBand:     pkgconfluence.ReadinessPossibleEntry,
		OIStatus:          pkgconfluence.OIStatusReady,
		MarketStatus:      pkgconfluence.MarketStatusOpen,
		RSI:               31,
		UpdatedAt:         time.Now().UTC(),
		Levels:            pkgconfluence.Levels{GammaFlip: 1650, NearestSupport: 1645, HasNearestSupport: true},
	}
	proc := &warmupSequenceProcessor{
		mockConfluenceProcessor: mockConfluenceProcessor{
			snapshots: map[string]*pkgconfluence.ConfluenceSnapshot{"SNDK": loading},
		},
		scored: scored,
	}
	svc := NewConfluenceService(proc)
	svc.oiWaitBudget = 50 * time.Millisecond

	resp1, err := svc.GetConfluenceSummary(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "SNDK"})
	if err != nil {
		t.Fatalf("first query: %v", err)
	}
	if resp1.Verdict == nil || resp1.Verdict.Buy == nil || resp1.Verdict.Buy.Score != 0 {
		t.Fatalf("first query should be partial/loading, got %+v", resp1.Verdict)
	}

	// Simulate background OI completing after the unary handler unsubscribed.
	proc.snapshots["SNDK"] = scored

	resp2, err := svc.GetConfluenceSummary(context.Background(), &confluencev1.GetConfluenceRequest{Ticker: "SNDK"})
	if err != nil {
		t.Fatalf("second query: %v", err)
	}
	if resp2.Verdict == nil || resp2.Verdict.Buy == nil || resp2.Verdict.Buy.Score != 58 {
		t.Fatalf("second query should be scored, got %+v", resp2.Verdict)
	}
}

type warmupSequenceProcessor struct {
	mockConfluenceProcessor
	scored *pkgconfluence.ConfluenceSnapshot
}

func (w *warmupSequenceProcessor) BootstrapInProgress(ticker string) bool {
	snap, ok := w.snapshots[ticker]
	return ok && snap != nil && snap.OIStatus == pkgconfluence.OIStatusLoading
}

func (w *warmupSequenceProcessor) RecomputeIfReady(_ context.Context, ticker string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	snap, ok := w.snapshots[ticker]
	if !ok || snap == nil || snap.OIStatus != pkgconfluence.OIStatusReady {
		return nil, false
	}
	return snap, true
}
