package list

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/arrgo/arrgo/internal/timeutil"
	"github.com/arrgo/arrgo/sdk/listingester"
)

// Config controls Manager construction.
type Config struct {
	Clock     timeutil.Clock
	Logger    *slog.Logger
	HealthTTL time.Duration
}

// Subscription captures the configuration for a managed list sync.
type Subscription struct {
	ID             string
	Provider       string
	List           listingester.ListRequest
	Mode           listingester.SyncMode
	Limit          int
	IdentityFields []string
	ResetCursor    bool
}

// SyncOutcome describes the result of a synchronization cycle.
type SyncOutcome struct {
	SubscriptionID string
	Provider       string
	ListID         string
	FullSync       bool
	HasMore        bool
	Cursor         string
	SyncedAt       time.Time
	Added          []listingester.Item
	Removed        []listingester.ItemRef
	Snapshot       []listingester.Item
	Stats          map[string]int
}

// Errors exposed by the manager.
var (
	ErrNoProvidersRegistered   = errors.New("list: no providers registered")
	ErrProviderRequired        = errors.New("list: provider name is required")
	ErrSubscriptionIDRequired  = errors.New("list: subscription id is required")
	ErrListIDRequired          = errors.New("list: list id is required")
	ErrProviderNotFound        = errors.New("list: provider not registered")
	ErrDuplicateProvider       = errors.New("list: provider with name already registered")
	ErrProviderUnhealthy       = errors.New("list: provider is unhealthy")
	ErrIdentityFieldResolution = errors.New("list: unable to resolve identity for item")
)

var defaultIdentityFields = []string{"trakt", "imdb", "tmdb", "tvdb", "musicbrainz", "lastfm", "slug", "upc", "title"}

// Manager coordinates synchronization across registered list providers.
type Manager struct {
	mu        sync.RWMutex
	providers []*registeredProvider
	byName    map[string]*registeredProvider
	history   map[string]*subscriptionState
	clock     timeutil.Clock
	logger    *slog.Logger
	healthTTL time.Duration
}

type registeredProvider struct {
	provider  listingester.Provider
	mu        sync.Mutex
	healthy   bool
	lastCheck time.Time
	lastErr   error
}

type subscriptionState struct {
	mu         sync.Mutex
	cursor     string
	lastSynced time.Time
	identity   []string
	items      map[string]listingester.Item
}

// NewManager constructs a Manager using the supplied configuration.
func NewManager(cfg Config) *Manager {
	clock := cfg.Clock
	if clock == nil {
		clock = timeutil.SystemClock{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	ttl := cfg.HealthTTL
	if ttl < 0 {
		ttl = 0
	}

	return &Manager{
		providers: make([]*registeredProvider, 0),
		byName:    make(map[string]*registeredProvider),
		history:   make(map[string]*subscriptionState),
		clock:     clock,
		logger:    logger,
		healthTTL: ttl,
	}
}

// RegisterProvider registers a provider with the manager.
func (m *Manager) RegisterProvider(provider listingester.Provider) error {
	if provider == nil {
		return fmt.Errorf("list: provider is required: %w", listingester.ErrUnsupportedList)
	}

	rp := &registeredProvider{provider: provider}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.byName[provider.Name()]; ok {
		return ErrDuplicateProvider
	}

	m.providers = append(m.providers, rp)
	m.byName[provider.Name()] = rp
	m.logger.Info("registered list provider", "provider", provider.Name())
	return nil
}

// Sync executes a synchronization cycle for the supplied subscription.
func (m *Manager) Sync(ctx context.Context, sub Subscription) (*SyncOutcome, error) {
	if sub.ID == "" {
		return nil, ErrSubscriptionIDRequired
	}
	if sub.Provider == "" {
		return nil, ErrProviderRequired
	}
	if sub.List.ListID == "" {
		return nil, ErrListIDRequired
	}

	identityFields := sub.IdentityFields
	if len(identityFields) == 0 {
		identityFields = defaultIdentityFields
	}

	m.mu.RLock()
	rp := m.byName[sub.Provider]
	m.mu.RUnlock()
	if rp == nil {
		if len(m.providers) == 0 {
			return nil, ErrNoProvidersRegistered
		}
		return nil, ErrProviderNotFound
	}

	if !m.ensureHealthy(ctx, rp) {
		return nil, fmt.Errorf("%w: %v", ErrProviderUnhealthy, rp.failure())
	}

	if !rp.provider.Supports(sub.List) {
		return nil, listingester.ErrUnsupportedList
	}

	state := m.ensureSubscriptionState(sub.ID)

	req := listingester.SyncRequest{
		List:  sub.List,
		Mode:  sub.Mode,
		Limit: sub.Limit,
	}
	if req.Mode != listingester.SyncModeFull {
		req.Mode = listingester.SyncModeDelta
	}

	state.mu.Lock()
	if sub.ResetCursor || req.Mode == listingester.SyncModeFull {
		req.Cursor = ""
	} else {
		req.Cursor = state.cursor
	}
	state.mu.Unlock()

	result, err := rp.provider.Sync(ctx, req)
	if err != nil {
		if errors.Is(err, listingester.ErrProviderUnavailable) {
			rp.markFailure(m.clock.Now(), err)
		}
		return nil, err
	}
	rp.markSuccess(m.clock.Now())

	if result == nil {
		result = &listingester.SyncResult{}
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	fullSync := req.Mode == listingester.SyncModeFull || sub.ResetCursor || state.cursor == "" || result.Reset
	if state.items == nil || fullSync {
		state.items = make(map[string]listingester.Item)
	}

	added := make([]listingester.Item, 0, len(result.Items))
	removed := make([]listingester.ItemRef, 0, len(result.Removed))

	for _, ref := range result.Removed {
		key := computeKey(ref.MediaType, ref.ExternalIDs, ref.Title, identityFields)
		if key == "" {
			continue
		}
		if _, ok := state.items[key]; ok {
			delete(state.items, key)
			removed = append(removed, cloneItemRef(ref))
		}
	}

	for _, item := range result.Items {
		if item.MediaType == "" {
			item.MediaType = sub.List.MediaType
		}
		key := computeKey(item.MediaType, item.ExternalIDs, item.Title, identityFields)
		if key == "" {
			continue
		}
		_, exists := state.items[key]
		state.items[key] = cloneItem(item)
		if !exists || fullSync {
			added = append(added, cloneItem(item))
		}
	}

	if result.NextCursor != "" || fullSync {
		state.cursor = result.NextCursor
	}
	state.lastSynced = m.clock.Now()
	state.identity = append([]string(nil), identityFields...)

	snapshot := make([]listingester.Item, 0, len(state.items))
	for _, item := range state.items {
		snapshot = append(snapshot, cloneItem(item))
	}
	sort.Slice(snapshot, func(i, j int) bool {
		left := strings.ToLower(snapshot[i].Title)
		right := strings.ToLower(snapshot[j].Title)
		if left == right {
			return computeKey(snapshot[i].MediaType, snapshot[i].ExternalIDs, snapshot[i].Title, identityFields) <
				computeKey(snapshot[j].MediaType, snapshot[j].ExternalIDs, snapshot[j].Title, identityFields)
		}
		return left < right
	})

	outcome := &SyncOutcome{
		SubscriptionID: sub.ID,
		Provider:       sub.Provider,
		ListID:         sub.List.ListID,
		FullSync:       fullSync,
		HasMore:        result.HasMore,
		Cursor:         state.cursor,
		SyncedAt:       state.lastSynced,
		Added:          added,
		Removed:        removed,
		Snapshot:       snapshot,
		Stats:          cloneStats(result.Stats),
	}

	return outcome, nil
}

func (m *Manager) ensureHealthy(ctx context.Context, rp *registeredProvider) bool {
	rp.mu.Lock()
	healthy := rp.healthy
	lastCheck := rp.lastCheck
	rp.mu.Unlock()

	if healthy && (m.healthTTL == 0 || m.clock.Now().Sub(lastCheck) < m.healthTTL) {
		return true
	}

	rp.mu.Lock()
	defer rp.mu.Unlock()

	if rp.healthy && (m.healthTTL == 0 || m.clock.Now().Sub(rp.lastCheck) < m.healthTTL) {
		return true
	}

	if err := rp.provider.CheckHealth(ctx); err != nil {
		rp.healthy = false
		rp.lastErr = err
		rp.lastCheck = m.clock.Now()
		m.logger.Warn("list provider health check failed", "provider", rp.provider.Name(), "error", err)
		return false
	}

	rp.healthy = true
	rp.lastErr = nil
	rp.lastCheck = m.clock.Now()
	return true
}

func (rp *registeredProvider) markFailure(now time.Time, err error) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	rp.healthy = false
	rp.lastErr = err
	rp.lastCheck = now
}

func (rp *registeredProvider) markSuccess(now time.Time) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	rp.healthy = true
	rp.lastErr = nil
	rp.lastCheck = now
}

func (rp *registeredProvider) failure() error {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.lastErr
}

func (m *Manager) ensureSubscriptionState(id string) *subscriptionState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.history[id]
	if !ok {
		state = &subscriptionState{items: make(map[string]listingester.Item)}
		m.history[id] = state
	}
	return state
}

func computeKey(mediaType string, ids map[string]string, title string, fields []string) string {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType == "" {
		mediaType = "unknown"
	}

	for _, field := range fields {
		field = strings.ToLower(strings.TrimSpace(field))
		if field == "" {
			continue
		}
		var value string
		if ids != nil {
			value = strings.TrimSpace(ids[field])
		}
		if value != "" {
			return fmt.Sprintf("%s:%s:%s", mediaType, field, strings.ToLower(value))
		}
	}

	value := strings.ToLower(strings.TrimSpace(title))
	if value == "" {
		return ""
	}
	return fmt.Sprintf("%s:title:%s", mediaType, value)
}

func cloneItem(item listingester.Item) listingester.Item {
	clone := listingester.Item{
		MediaType:   item.MediaType,
		Title:       item.Title,
		AddedAt:     item.AddedAt,
		ExternalIDs: cloneStringMap(item.ExternalIDs),
	}
	if len(item.Metadata) > 0 {
		clone.Metadata = cloneMetadata(item.Metadata)
	}
	return clone
}

func cloneItemRef(ref listingester.ItemRef) listingester.ItemRef {
	return listingester.ItemRef{
		MediaType:   ref.MediaType,
		Title:       ref.Title,
		ExternalIDs: cloneStringMap(ref.ExternalIDs),
	}
}

func cloneStats(stats map[string]int) map[string]int {
	if len(stats) == 0 {
		return nil
	}
	clone := make(map[string]int, len(stats))
	for k, v := range stats {
		clone[k] = v
	}
	return clone
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	clone := make(map[string]string, len(src))
	for k, v := range src {
		clone[k] = v
	}
	return clone
}

func cloneMetadata(src map[string]any) map[string]any {
	clone := make(map[string]any, len(src))
	for k, v := range src {
		clone[k] = v
	}
	return clone
}
