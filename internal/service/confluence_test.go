package service

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	confluencev1 "github.com/ekinolik/jax/api/proto/confluence/v1"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type mockConfluenceProcessor struct {
	snapshots       map[string]*pkgconfluence.ConfluenceSnapshot
	onSubErr        error
	watchErr        error
	subscribeCount  int
	unsubscribeCount int
}

func (m *mockConfluenceProcessor) GetSnapshot(ticker string) (*pkgconfluence.ConfluenceSnapshot, bool) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	snap, ok := m.snapshots[ticker]
	return snap, ok
}

func (m *mockConfluenceProcessor) OnSubscribe(ctx context.Context, ticker string, now time.Time) error {
	m.subscribeCount++
	return m.onSubErr
}

func (m *mockConfluenceProcessor) OnUnsubscribe(ticker string) {
	m.unsubscribeCount++
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

func TestGetConfluence_cachedSnapshot(t *testing.T) {
	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:        "NVDA",
		Score:         55,
		ReadinessBand: pkgconfluence.ReadinessPossibleEntry,
		UpdatedAt:     time.Now().UTC(),
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

func TestGetConfluence_waitTimeout(t *testing.T) {
	proc := &mockConfluenceProcessor{snapshots: map[string]*pkgconfluence.ConfluenceSnapshot{}}
	svc := NewConfluenceService(proc)
	svc.timeout = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := svc.GetConfluence(ctx, &confluencev1.GetConfluenceRequest{Ticker: "NVDA"})
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
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

func (b *blockingMockProcessor) Watch(ctx context.Context, ticker string) (<-chan *pkgconfluence.ConfluenceSnapshot, func(), error) {
	return b.ch, func() {}, nil
}

func TestGetConfluence_subscribeThenSnapshot(t *testing.T) {
	proc := &delayedMockProcessor{}
	svc := NewConfluenceService(proc)
	svc.timeout = 2 * time.Second

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
		UpdatedAt:     time.Now().UTC(),
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

func (d *delayedMockProcessor) Watch(context.Context, string) (<-chan *pkgconfluence.ConfluenceSnapshot, func(), error) {
	return nil, nil, io.EOF
}
