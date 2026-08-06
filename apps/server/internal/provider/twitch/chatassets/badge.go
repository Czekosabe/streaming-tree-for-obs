package chatassets

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	twitch "github.com/streaming-tree/server/internal/provider/twitch"
)

// cacheTTL is how long a fetched badge catalog is trusted before a fresh
// fetch is made - see the Stage 9 addendum's "Cache behavior" section: an
// hour is generous but bounded, since badge catalogs change rarely.
const cacheTTL = time.Hour

// maxCacheEntries bounds total cached catalogs ("global" plus one per
// distinct broadcaster channel resolved so far) - eviction is unordered
// once the bound is hit, not strict LRU, which is an acceptable tradeoff
// for a cache this small and this rarely written.
const maxCacheEntries = 64

// globalCacheKey is the cache key for the provider-wide badge catalog, as
// opposed to a broadcaster's own provider user id.
const globalCacheKey = "global"

// Image is one badge version's resolved image, at Twitch's three
// documented sizes - a caller picks whichever size fits its layout, or
// builds a srcset from all three.
type Image struct {
	URL1x string
	URL2x string
	URL4x string
}

type versionKey struct{ setID, version string }

type catalog struct {
	versions  map[versionKey]Image
	expiresAt time.Time
}

type inFlightCall struct {
	done   chan struct{}
	result catalog
	err    error
}

// Resolver resolves a Twitch chat badge (set id + version, as already
// normalized on internal/operatorchat.Badge) to an Image, backed by a
// bounded, TTL'd, single-flight cache - mirroring
// internal/domain/account.Service's own hand-rolled single-flight pattern
// (see that type's singleFlightRefresh).
//
// A badge that cannot be resolved (cache miss during a failed fetch, an
// unknown set/version) is reported via ok=false - callers omit it from the
// rendered badge list rather than blocking or discarding the message it
// belongs to.
type Resolver struct {
	client   *twitch.Client
	accounts *account.Service
	now      func() time.Time

	mu       sync.Mutex
	cache    map[string]catalog
	inFlight map[string]*inFlightCall
}

// NewResolver builds a Resolver.
func NewResolver(client *twitch.Client, accounts *account.Service, now func() time.Time) *Resolver {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Resolver{
		client: client, accounts: accounts, now: now,
		cache: make(map[string]catalog), inFlight: make(map[string]*inFlightCall),
	}
}

// ResolveBadge resolves one badge reference for the channel behind
// accountID (a connected Twitch account id) - checking that channel's own
// badge catalog first, then falling back to the global catalog, per the
// Stage 9 addendum's documented (inferred) override order.
func (r *Resolver) ResolveBadge(ctx context.Context, accountID, setID, version string) (Image, bool) {
	acc, err := r.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return Image{}, false
	}
	key := versionKey{setID, version}

	channelCatalog, err := r.getCatalog(ctx, acc.ProviderUserID, accountID, func(ctx context.Context, token, clientID string) ([]twitch.ChatBadgeSet, error) {
		return r.client.GetChannelChatBadges(ctx, acc.ProviderUserID, token, clientID)
	})
	if err == nil {
		if img, ok := channelCatalog.versions[key]; ok {
			return img, true
		}
	}

	globalCatalog, err := r.getCatalog(ctx, globalCacheKey, accountID, func(ctx context.Context, token, clientID string) ([]twitch.ChatBadgeSet, error) {
		return r.client.GetGlobalChatBadges(ctx, token, clientID)
	})
	if err == nil {
		if img, ok := globalCatalog.versions[key]; ok {
			return img, true
		}
	}
	return Image{}, false
}

type fetchFunc func(ctx context.Context, accessToken, clientID string) ([]twitch.ChatBadgeSet, error)

func (r *Resolver) getCatalog(ctx context.Context, cacheKey, accountID string, fetch fetchFunc) (catalog, error) {
	r.mu.Lock()
	if c, ok := r.cache[cacheKey]; ok && r.now().Before(c.expiresAt) {
		r.mu.Unlock()
		return c, nil
	}
	if call, inFlight := r.inFlight[cacheKey]; inFlight {
		r.mu.Unlock()
		<-call.done
		return call.result, call.err
	}
	call := &inFlightCall{done: make(chan struct{})}
	r.inFlight[cacheKey] = call
	r.mu.Unlock()

	result, err := r.fetchCatalog(ctx, accountID, fetch)
	call.result, call.err = result, err
	close(call.done)

	r.mu.Lock()
	delete(r.inFlight, cacheKey)
	if err == nil {
		r.cache[cacheKey] = result
		r.evictIfNeededLocked()
	}
	r.mu.Unlock()

	return result, err
}

// evictIfNeededLocked keeps the cache bounded - see maxCacheEntries's own
// doc comment for why eviction order is not strict LRU. Callers must hold
// r.mu.
func (r *Resolver) evictIfNeededLocked() {
	for len(r.cache) > maxCacheEntries {
		for k := range r.cache {
			delete(r.cache, k)
			break
		}
	}
}

func (r *Resolver) fetchCatalog(ctx context.Context, accountID string, fetch fetchFunc) (catalog, error) {
	acc, err := r.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return catalog{}, err
	}
	clientID, err := r.accounts.EffectiveClientID(ctx, acc.ProviderID)
	if err != nil {
		return catalog{}, err
	}

	var (
		sets    []twitch.ChatBadgeSet
		callErr error
	)
	err = r.accounts.WithFreshToken(ctx, accountID, func(token string) (bool, error) {
		fetched, fetchErr := fetch(ctx, token, clientID)
		if fetchErr != nil {
			if errors.Is(fetchErr, twitch.ErrUnauthorized) {
				return true, nil
			}
			callErr = fetchErr
			return false, nil
		}
		sets = fetched
		return false, nil
	})
	if err != nil {
		return catalog{}, err
	}
	if callErr != nil {
		return catalog{}, callErr
	}

	versions := make(map[versionKey]Image, len(sets)*2)
	for _, set := range sets {
		for _, v := range set.Versions {
			versions[versionKey{set.SetID, v.ID}] = Image{URL1x: v.ImageURL1x, URL2x: v.ImageURL2x, URL4x: v.ImageURL4x}
		}
	}
	return catalog{versions: versions, expiresAt: r.now().Add(cacheTTL)}, nil
}
