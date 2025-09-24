package static

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/arrgo/arrgo/sdk/listingester"
)

// List models a static upstream list for deterministic tests and in-memory providers.
type List struct {
	Kind      listingester.ListKind
	MediaType string
	Items     []Item
	Removals  []Removal
}

// Item describes a single entry in the static list.
type Item struct {
	Title       string
	MediaType   string
	ExternalIDs map[string]string
	AddedAt     time.Time
	Metadata    map[string]any
}

// Removal describes an entry removal event.
type Removal struct {
	Title       string
	MediaType   string
	ExternalIDs map[string]string
	RemovedAt   time.Time
}

// Provider implements listingester.Provider using in-memory data sets.
type Provider struct {
	name  string
	mu    sync.RWMutex
	lists map[string]*List
	err   error
}

// New constructs a Provider backed by the supplied lists.
func New(name string, lists map[string]List) *Provider {
	cloned := make(map[string]*List, len(lists))
	for id, list := range lists {
		cloned[id] = cloneList(list)
	}
	return &Provider{name: name, lists: cloned}
}

// Name returns the provider name.
func (p *Provider) Name() string { return p.name }

// Supports reports whether the request is backed by a static list.
func (p *Provider) Supports(req listingester.ListRequest) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	list, ok := p.lists[req.ListID]
	if !ok {
		return false
	}
	if req.MediaType != "" && !equalsFold(req.MediaType, list.MediaType) {
		return false
	}
	if req.Kind != "" && list.Kind != "" && req.Kind != list.Kind {
		return false
	}
	return true
}

// CheckHealth returns the configured error, if any.
func (p *Provider) CheckHealth(context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.err
}

// SetHealthError updates the health error returned by CheckHealth.
func (p *Provider) SetHealthError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

// Sync returns either a full snapshot or delta for the static list.
func (p *Provider) Sync(ctx context.Context, req listingester.SyncRequest) (*listingester.SyncResult, error) {
	p.mu.RLock()
	list, ok := p.lists[req.List.ListID]
	p.mu.RUnlock()
	if !ok {
		return nil, listingester.ErrUnsupportedList
	}
	if req.Mode != listingester.SyncModeFull {
		req.Mode = listingester.SyncModeDelta
	}

	cursorTime, _ := time.Parse(time.RFC3339Nano, req.Cursor)
	latestCursor := cursorTime
	items := make([]listingester.Item, 0, len(list.Items))
	removals := make([]listingester.ItemRef, 0, len(list.Removals))

	if req.Mode == listingester.SyncModeFull || req.Cursor == "" {
		for _, item := range list.Items {
			converted := convertItem(item)
			items = append(items, converted)
			if item.AddedAt.After(latestCursor) {
				latestCursor = item.AddedAt
			}
		}
	} else {
		for _, item := range list.Items {
			if item.AddedAt.After(cursorTime) {
				converted := convertItem(item)
				items = append(items, converted)
				if item.AddedAt.After(latestCursor) {
					latestCursor = item.AddedAt
				}
			}
		}
		for _, removal := range list.Removals {
			if removal.RemovedAt.After(cursorTime) {
				removals = append(removals, convertRemoval(removal))
				if removal.RemovedAt.After(latestCursor) {
					latestCursor = removal.RemovedAt
				}
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].AddedAt.Before(items[j].AddedAt)
	})
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}

	if req.Mode == listingester.SyncModeFull || req.Cursor == "" {
		removals = nil
	}

	nextCursor := req.Cursor
	if len(items) > 0 || len(removals) > 0 {
		nextCursor = latestCursor.UTC().Format(time.RFC3339Nano)
	}

	hasMore := req.Limit > 0 && len(items) == req.Limit

	return &listingester.SyncResult{
		Items:      items,
		Removed:    removals,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Reset:      req.Mode == listingester.SyncModeFull || req.Cursor == "",
	}, nil
}

func cloneList(src List) *List {
	cloned := &List{
		Kind:      src.Kind,
		MediaType: src.MediaType,
		Items:     make([]Item, len(src.Items)),
		Removals:  make([]Removal, len(src.Removals)),
	}
	copy(cloned.Items, src.Items)
	copy(cloned.Removals, src.Removals)
	return cloned
}

func convertItem(item Item) listingester.Item {
	converted := listingester.Item{
		Title:       item.Title,
		MediaType:   item.MediaType,
		ExternalIDs: cloneStringMap(item.ExternalIDs),
		AddedAt:     item.AddedAt,
	}
	if len(item.Metadata) > 0 {
		converted.Metadata = cloneMetadata(item.Metadata)
	}
	if converted.MediaType == "" {
		converted.MediaType = item.MediaType
	}
	return converted
}

func convertRemoval(removal Removal) listingester.ItemRef {
	return listingester.ItemRef{
		Title:       removal.Title,
		MediaType:   removal.MediaType,
		ExternalIDs: cloneStringMap(removal.ExternalIDs),
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(src))
	for k, v := range src {
		cloned[k] = v
	}
	return cloned
}

func cloneMetadata(src map[string]any) map[string]any {
	cloned := make(map[string]any, len(src))
	for k, v := range src {
		cloned[k] = v
	}
	return cloned
}

func equalsFold(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}
