// package manifest handles fetching and caching the dotfile registry
package manifest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Fetcher handles downloading and caching the manifest
type Fetcher struct {
	url       string
	cachePath string
	client    *http.Client
}

// NewFetcher creates a manifest fetcher
// url is where to fetch the manifest from
// cachePath is where to cache it locally
func NewFetcher(url, cachePath string) *Fetcher {
	return &Fetcher{
		url:       url,
		cachePath: cachePath,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Fetch downloads the latest manifest from the remote url
// tries remote first, falls back to cache if network fails
func (f *Fetcher) Fetch(ctx context.Context) (*Manifest, error) {
	// try to fetch the remote manifest
	manifest, err := f.fetchRemote(ctx)
	if err != nil {
		// no worries, let's check the cache
		cached, cacheErr := f.loadCache()
		if cacheErr != nil {
			// both failed - return the original error
			return nil, fmt.Errorf("couldn't fetch manifest: %w (cache also unavailable: %v)", err, cacheErr)
		}
		return cached, nil
	}

	// save to cache for next time
	if err := f.saveCache(manifest); err != nil {
		// not critical, just log and continue
		// (we should add proper logging later)
		_ = err
	}

	return manifest, nil
}

// fetchRemote downloads the manifest from the remote url
func (f *Fetcher) fetchRemote(ctx context.Context) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// read and parse the json
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("couldn't read response: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("couldn't parse manifest: %w", err)
	}

	return &manifest, nil
}

// loadCache loads the manifest from disk cache
func (f *Fetcher) loadCache() (*Manifest, error) {
	data, err := os.ReadFile(f.cachePath)
	if err != nil {
		return nil, fmt.Errorf("couldn't read cache: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("couldn't parse cached manifest: %w", err)
	}

	return &manifest, nil
}

// saveCache writes the manifest to disk cache
func (f *Fetcher) saveCache(manifest *Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("couldn't marshal manifest: %w", err)
	}

	if err := os.WriteFile(f.cachePath, data, 0644); err != nil {
		return fmt.Errorf("couldn't write cache: %w", err)
	}

	return nil
}

// NeedRefresh checks if the cached manifest is stale
// returns true if we should fetch a new one
func (f *Fetcher) NeedRefresh(maxAge time.Duration) bool {
	info, err := os.Stat(f.cachePath)
	if err != nil {
		// cache doesn't exist or can't read it
		return true
	}

	age := time.Since(info.ModTime())
	return age > maxAge
}
