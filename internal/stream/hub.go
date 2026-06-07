package stream

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	massivews "github.com/massive-com/client-go/v2/websocket"
	wsmodels "github.com/massive-com/client-go/v2/websocket/models"
)

const (
	reconnectBaseDelay = time.Second
	reconnectMaxDelay  = 60 * time.Second
)

// SpotTick is the latest trade price for a symbol.
type SpotTick struct {
	Price     float64
	Timestamp time.Time
}

// SpotHandler receives spot updates for subscribed tickers.
type SpotHandler func(ticker string, tick SpotTick)

// Hub maintains real-time spot state from Massive stock trade WebSocket (T.*).
type Hub struct {
	apiKey string
	client *massivews.Client

	mu           sync.RWMutex
	spots        map[string]SpotTick
	subs         map[string]int
	spotHandlers []SpotHandler
	started      bool

	runCtx    context.Context
	runCancel context.CancelFunc
	runWg     sync.WaitGroup
}

// NewHub creates a stream hub configured for Massive RealTime stock trades.
func NewHub(apiKey string) (*Hub, error) {
	client, err := massivews.New(massivews.Config{
		APIKey: apiKey,
		Feed:   massivews.RealTime,
		Market: massivews.Stocks,
	})
	if err != nil {
		return nil, fmt.Errorf("create stream hub client: %w", err)
	}

	return &Hub{
		apiKey: apiKey,
		client: client,
		spots:  make(map[string]SpotTick),
		subs:   make(map[string]int),
	}, nil
}

// Start connects to Massive and begins processing trade messages with auto-reconnect.
func (h *Hub) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return nil
	}

	h.runCtx, h.runCancel = context.WithCancel(ctx)
	h.started = true
	h.runWg.Add(1)
	go func() {
		defer h.runWg.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[stream] hub panic: %v", r)
				h.mu.Lock()
				h.started = false
				h.mu.Unlock()
			}
		}()
		h.runSupervisor(h.runCtx)
	}()
	return nil
}

// Stop closes the WebSocket connection and waits for the run loop to exit.
func (h *Hub) Stop() {
	h.mu.Lock()
	if !h.started {
		h.mu.Unlock()
		return
	}
	cancel := h.runCancel
	h.started = false
	h.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	h.runWg.Wait()
	h.closeClient()
}

// Subscribe registers interest in a ticker and subscribes to T.{ticker} when first referenced.
func (h *Hub) Subscribe(ticker string) error {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if err := pkgconfluence.ValidateTicker(ticker); err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if count, ok := h.subs[ticker]; ok && count > 0 {
		h.subs[ticker] = count + 1
		return nil
	}

	if err := h.client.Subscribe(massivews.StocksTrades, ticker); err != nil {
		return fmt.Errorf("subscribe trades for %s: %w", ticker, err)
	}
	h.subs[ticker] = 1
	return nil
}

// Unsubscribe drops interest in a ticker and unsubscribes when ref-count reaches zero.
func (h *Hub) Unsubscribe(ticker string) error {
	ticker = pkgconfluence.NormalizeTicker(ticker)

	h.mu.Lock()
	defer h.mu.Unlock()

	count, ok := h.subs[ticker]
	if !ok || count == 0 {
		return nil
	}
	if count > 1 {
		h.subs[ticker] = count - 1
		return nil
	}

	if err := h.client.Unsubscribe(massivews.StocksTrades, ticker); err != nil {
		return fmt.Errorf("unsubscribe trades for %s: %w", ticker, err)
	}
	delete(h.subs, ticker)
	delete(h.spots, ticker)
	return nil
}

// InjectSpot sets spot state and invokes registered handlers (for tests and REST fallback).
func (h *Hub) InjectSpot(ticker string, tick SpotTick) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	h.mu.Lock()
	h.spots[ticker] = tick
	handlers := append([]SpotHandler(nil), h.spotHandlers...)
	h.mu.Unlock()
	for _, fn := range handlers {
		fn(ticker, tick)
	}
}

// OnSpotUpdate registers a callback invoked after each trade tick update.
func (h *Hub) OnSpotUpdate(fn SpotHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.spotHandlers = append(h.spotHandlers, fn)
}

// GetSpot returns the latest spot tick for a ticker, if available.
func (h *Hub) GetSpot(ticker string) (SpotTick, bool) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	h.mu.RLock()
	defer h.mu.RUnlock()
	tick, ok := h.spots[ticker]
	return tick, ok
}

func (h *Hub) runSupervisor(ctx context.Context) {
	delay := reconnectBaseDelay
	for {
		if ctx.Err() != nil {
			return
		}
		if err := h.runSession(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[stream] hub disconnected: %v; reconnecting in %s", err, delay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if delay < reconnectMaxDelay {
				delay *= 2
				if delay > reconnectMaxDelay {
					delay = reconnectMaxDelay
				}
			}
			continue
		}
		delay = reconnectBaseDelay
	}
}

func (h *Hub) runSession(ctx context.Context) error {
	if err := h.ensureConnected(ctx); err != nil {
		return err
	}
	return h.run(ctx)
}

func (h *Hub) ensureConnected(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := h.client.Connect(); err != nil {
		if recreateErr := h.recreateClient(); recreateErr != nil {
			return fmt.Errorf("connect stream hub: %w (recreate: %v)", err, recreateErr)
		}
		if err := h.client.Connect(); err != nil {
			return fmt.Errorf("connect stream hub after recreate: %w", err)
		}
	}
	return h.resubscribeAll()
}

func (h *Hub) recreateClient() error {
	h.closeClient()

	client, err := massivews.New(massivews.Config{
		APIKey: h.apiKey,
		Feed:   massivews.RealTime,
		Market: massivews.Stocks,
	})
	if err != nil {
		return err
	}
	h.client = client
	return nil
}

func (h *Hub) closeClient() {
	if h.client != nil {
		h.client.Close()
	}
}

func (h *Hub) resubscribeAll() error {
	h.mu.RLock()
	tickers := make([]string, 0, len(h.subs))
	for ticker, count := range h.subs {
		if count > 0 {
			tickers = append(tickers, ticker)
		}
	}
	h.mu.RUnlock()

	for _, ticker := range tickers {
		if err := h.client.Subscribe(massivews.StocksTrades, ticker); err != nil {
			return fmt.Errorf("resubscribe trades for %s: %w", ticker, err)
		}
	}
	return nil
}

func (h *Hub) run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-h.client.Error():
			if err != nil {
				return fmt.Errorf("websocket error: %w", err)
			}
		case out, ok := <-h.client.Output():
			if !ok {
				return fmt.Errorf("websocket output closed")
			}
			h.handleMessage(out)
		}
	}
}

func (h *Hub) handleMessage(msg any) {
	trade, ok := msg.(wsmodels.EquityTrade)
	if !ok {
		return
	}
	if trade.Symbol == "" || trade.Price <= 0 || math.IsNaN(trade.Price) || math.IsInf(trade.Price, 0) {
		return
	}

	symbol := pkgconfluence.NormalizeTicker(trade.Symbol)
	tick := SpotTick{
		Price:     trade.Price,
		Timestamp: time.UnixMilli(trade.Timestamp).UTC(),
	}

	h.mu.Lock()
	h.spots[symbol] = tick
	handlers := append([]SpotHandler(nil), h.spotHandlers...)
	h.mu.Unlock()

	for _, fn := range handlers {
		fn(symbol, tick)
	}
}
