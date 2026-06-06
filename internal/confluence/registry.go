package confluence

import (
	"fmt"
	"sync"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

// TickerEntry tracks active confluence state for one ticker.
type TickerEntry struct {
	SubscriberCount int
	OIState         pkgconfluence.OIStatus
	LastGreeksAt    time.Time
	IdleSince       time.Time
}

// Registry tracks active tickers with reference counting for shared resources.
type Registry struct {
	mu        sync.Mutex
	maxActive int
	entries   map[string]*TickerEntry
}

// NewRegistry creates a registry with the configured active ticker cap.
func NewRegistry(maxActive int) *Registry {
	return &Registry{
		maxActive: maxActive,
		entries:   make(map[string]*TickerEntry),
	}
}

// Subscribe increments the subscriber count for a ticker.
func (r *Registry) Subscribe(ticker string) (*TickerEntry, error) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	if err := pkgconfluence.ValidateTicker(ticker); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if entry, ok := r.entries[ticker]; ok && entry.SubscriberCount > 0 {
		entry.SubscriberCount++
		entry.IdleSince = time.Time{}
		return entry, nil
	}

	if r.activeCountLocked() >= r.maxActive {
		return nil, fmt.Errorf("max active tickers (%d) reached", r.maxActive)
	}

	entry := &TickerEntry{OIState: pkgconfluence.OIStatusLoading}
	r.entries[ticker] = entry
	entry.SubscriberCount = 1
	return entry, nil
}

// Unsubscribe decrements the subscriber count and marks idle time when last client leaves.
func (r *Registry) Unsubscribe(ticker string) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[ticker]
	if !ok {
		return
	}
	entry.SubscriberCount--
	if entry.SubscriberCount <= 0 {
		entry.SubscriberCount = 0
		entry.IdleSince = time.Now().UTC()
	}
}

// Remove drops a ticker from the registry after cleanup.
func (r *Registry) Remove(ticker string) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, ticker)
}

// Get returns the registry entry for a ticker.
func (r *Registry) Get(ticker string) (*TickerEntry, bool) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[ticker]
	return entry, ok
}

// ActiveTickers returns tickers with at least one subscriber.
func (r *Registry) ActiveTickers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var tickers []string
	for ticker, entry := range r.entries {
		if entry.SubscriberCount > 0 {
			tickers = append(tickers, ticker)
		}
	}
	return tickers
}

// SetOIState updates OI load status for a ticker.
func (r *Registry) SetOIState(ticker string, state pkgconfluence.OIStatus) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.entries[ticker]; ok {
		entry.OIState = state
	}
}

// MarkGreeksRefresh records the latest greeks refresh timestamp.
func (r *Registry) MarkGreeksRefresh(ticker string, at time.Time) {
	ticker = pkgconfluence.NormalizeTicker(ticker)
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.entries[ticker]; ok {
		entry.LastGreeksAt = at
	}
}

func (r *Registry) activeCountLocked() int {
	count := 0
	for _, entry := range r.entries {
		if entry.SubscriberCount > 0 {
			count++
		}
	}
	return count
}
