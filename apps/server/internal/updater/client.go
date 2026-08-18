package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RepoOwner and RepoName are the fixed, non-configurable production
// update source - see docs/updater.md §1/§15. Nothing in this package
// exposes a way to change them at runtime; a different source can only
// be reached by calling newClient with a different baseURL, which is
// unexported and used only by this package's own tests.
const (
	RepoOwner = "Czekosabe"
	RepoName  = "streaming-tree-for-obs"

	// RepositoryURL is the canonical repository URL, embedded in the
	// User-Agent this client sends (docs/updater.md §2/§8).
	RepositoryURL = "https://github.com/Czekosabe/streaming-tree-for-obs"
)

const (
	productionAPIBaseURL = "https://api.github.com"

	// apiVersionHeader/apiVersionValue mirror GitHub's own current
	// documented recommendation (docs/updater.md §2, researched
	// 2026-08-18) - reviewed whenever this contract is next revisited,
	// not discovered dynamically at runtime.
	apiVersionHeader = "X-GitHub-Api-Version"
	apiVersionValue  = "2026-03-10"
	acceptHeader     = "application/vnd.github+json"

	// metadataRequestTimeout bounds a single latest-release metadata
	// request (docs/updater.md §8) - short, since this is a small JSON
	// response, never the installer download itself.
	metadataRequestTimeout = 15 * time.Second

	// maxMetadataResponseBytes bounds the latest-release JSON response -
	// generous for any real release's metadata (including a long body),
	// far below anything resembling abuse.
	maxMetadataResponseBytes = 2 * 1024 * 1024

	// manifestAssetName is the fixed, hard-coded expected name of the
	// project-controlled release manifest asset (docs/updater.md §5) -
	// never discovered by pattern-matching.
	manifestAssetName = "streaming-tree-release.json"

	// maxManifestResponseBytes bounds the manifest asset download -
	// this is a tiny JSON document, generously bounded well below any
	// real size.
	maxManifestResponseBytes = 256 * 1024
)

// Asset is one GitHub release asset, the fields this project actually
// uses (docs/updater.md §2).
type Asset struct {
	ID                 int64
	Name               string
	Size               int64
	BrowserDownloadURL string
	APIURL             string
	ContentType        string
	Digest             string // "sha256:<hex>" when GitHub reports one, else "".
}

// Release is a GitHub release, the fields this project actually uses
// (docs/updater.md §2).
type Release struct {
	ID          int64
	TagName     string
	Name        string
	Draft       bool
	Prerelease  bool
	Body        string
	PublishedAt time.Time
	Assets      []Asset
}

// AssetByName returns the release's asset matching name exactly - never
// substring, never case-insensitive (docs/updater.md §6). Returns
// ErrAssetNotFound for zero matches and ErrAssetAmbiguous for more than
// one - a caller never picks between candidates.
func (r Release) AssetByName(name string) (Asset, error) {
	var found *Asset
	for i := range r.Assets {
		if r.Assets[i].Name != name {
			continue
		}
		if found != nil {
			return Asset{}, fmt.Errorf("%w: %q", ErrAssetAmbiguous, name)
		}
		found = &r.Assets[i]
	}
	if found == nil {
		return Asset{}, fmt.Errorf("%w: %q", ErrAssetNotFound, name)
	}
	return *found, nil
}

// ManifestAsset returns the release's project-controlled release-
// manifest asset (docs/updater.md §5).
func (r Release) ManifestAsset() (Asset, error) {
	return r.AssetByName(manifestAssetName)
}

// wireRelease/wireAsset are the exact GitHub API JSON shapes (unexported
// - Release/Asset above are this package's own stable public types, so a
// future GitHub API field this project does not use never has to
// propagate outward).
type wireRelease struct {
	ID          int64       `json:"id"`
	TagName     string      `json:"tag_name"`
	Name        string      `json:"name"`
	Draft       bool        `json:"draft"`
	Prerelease  bool        `json:"prerelease"`
	Body        string      `json:"body"`
	PublishedAt time.Time   `json:"published_at"`
	Assets      []wireAsset `json:"assets"`
}

type wireAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
	ContentType        string `json:"content_type"`
	Digest             string `json:"digest"`
}

func (w wireRelease) toRelease() Release {
	assets := make([]Asset, len(w.Assets))
	for i, a := range w.Assets {
		assets[i] = Asset{
			ID:                 a.ID,
			Name:               a.Name,
			Size:               a.Size,
			BrowserDownloadURL: a.BrowserDownloadURL,
			APIURL:             a.URL,
			ContentType:        a.ContentType,
			Digest:             a.Digest,
		}
	}
	return Release{
		ID: w.ID, TagName: w.TagName, Name: w.Name,
		Draft: w.Draft, Prerelease: w.Prerelease,
		Body: w.Body, PublishedAt: w.PublishedAt, Assets: assets,
	}
}

// Client is a bounded GitHub Releases API client (docs/updater.md §8).
// Zero value is not usable - construct with NewClient.
type Client struct {
	http *http.Client
	// download is a second client with no fixed wall-clock Timeout,
	// used only for the installer artifact transfer - that transfer is
	// bounded by byte count (docs/updater.md §13) and the caller's own
	// context deadline, not by a short metadata-request timeout.
	download  *http.Client
	baseURL   string
	userAgent string
}

// NewClient builds the production client, fixed to the canonical
// repository's real GitHub API host. installedVersion is embedded in
// the User-Agent only (docs/updater.md §2) - never used for anything
// security-relevant.
func NewClient(installedVersion string) *Client {
	return newClient(productionAPIBaseURL, installedVersion)
}

// newClient is the internal constructor tests in this package use to
// point at a local httptest server instead of the real GitHub API -
// unexported, so no production code path can ever reach a
// non-canonical host (docs/updater.md §1/§15).
func newClient(baseURL, installedVersion string) *Client {
	return &Client{
		http:      &http.Client{Timeout: metadataRequestTimeout},
		download:  &http.Client{},
		baseURL:   strings.TrimRight(baseURL, "/"),
		userAgent: fmt.Sprintf("StreamingTreeForOBS/%s (+%s)", installedVersion, RepositoryURL),
	}
}

// LatestReleaseResult is the outcome of FetchLatestRelease.
type LatestReleaseResult struct {
	// Release is nil when NotModified is true.
	Release *Release
	// ETag is the response's own ETag, to pass as etag on the next call.
	// Empty when the server sent none.
	ETag string
	// NotModified is true on a 304 response (docs/updater.md §9) - a
	// normal, successful "no change" outcome, never an error.
	NotModified bool
}

// FetchLatestRelease calls GET /repos/{owner}/{repo}/releases/latest.
// etag, when non-empty, is sent as If-None-Match (docs/updater.md §9).
func (c *Client) FetchLatestRelease(ctx context.Context, etag string) (LatestReleaseResult, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.baseURL, RepoOwner, RepoName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LatestReleaseResult{}, fmt.Errorf("%w: %s", ErrRequestFailed, err)
	}
	c.setCommonHeaders(req)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return LatestReleaseResult{}, fmt.Errorf("%w: %s", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	respETag := resp.Header.Get("ETag")

	if resp.StatusCode == http.StatusNotModified {
		return LatestReleaseResult{NotModified: true, ETag: respETag}, nil
	}

	if isRateLimited(resp) {
		return LatestReleaseResult{}, ErrRateLimited
	}

	if resp.StatusCode != http.StatusOK {
		return LatestReleaseResult{}, fmt.Errorf("%w: unexpected status %d", ErrRequestFailed, resp.StatusCode)
	}

	body, err := readBounded(resp.Body, maxMetadataResponseBytes)
	if err != nil {
		return LatestReleaseResult{}, err
	}

	var wire wireRelease
	if err := json.Unmarshal(body, &wire); err != nil {
		return LatestReleaseResult{}, fmt.Errorf("%w: malformed release json: %s", ErrRequestFailed, err)
	}

	release := wire.toRelease()
	return LatestReleaseResult{Release: &release, ETag: respETag}, nil
}

// FetchManifest downloads and strictly parses the release's own
// project-controlled manifest asset - it does not call manifest.Validate,
// which requires the release's tag and is the caller's responsibility
// (docs/updater.md §5).
func (c *Client) FetchManifest(ctx context.Context, asset Asset) ([]byte, error) {
	return c.downloadBounded(ctx, asset, maxManifestResponseBytes)
}

// setCommonHeaders applies the fixed Accept/API-version/User-Agent
// headers every request carries (docs/updater.md §2/§8).
func (c *Client) setCommonHeaders(req *http.Request) {
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set(apiVersionHeader, apiVersionValue)
	req.Header.Set("User-Agent", c.userAgent)
}

// assetRequest builds a GET request for asset's own download location -
// its API resource URL preferred over browser_download_url
// (docs/updater.md §6).
func (c *Client) assetRequest(ctx context.Context, asset Asset) (*http.Request, error) {
	url := asset.APIURL
	if url == "" {
		url = asset.BrowserDownloadURL
	}
	if url == "" {
		return nil, fmt.Errorf("%w: asset %q has no download url", ErrRequestFailed, asset.Name)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set(apiVersionHeader, apiVersionValue)
	req.Header.Set("User-Agent", c.userAgent)
	return req, nil
}

// downloadBounded fetches an asset's raw bytes entirely into memory,
// bounded by maxBytes - used only for the small manifest asset, never
// the installer artifact (see DownloadAssetTo for that).
func (c *Client) downloadBounded(ctx context.Context, asset Asset, maxBytes int64) ([]byte, error) {
	req, err := c.assetRequest(ctx, asset)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	if isRateLimited(resp) {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status %d downloading %q", ErrRequestFailed, resp.StatusCode, asset.Name)
	}

	return readBounded(resp.Body, maxBytes)
}

// downloadChunkSize is the buffer size DownloadAssetTo streams with.
const downloadChunkSize = 32 * 1024

// DownloadAssetTo streams asset's bytes to dest (a real file, in
// production), bounded by maxBytes, and returns the lowercase-hex
// SHA-256 of the downloaded bytes - computed in the same pass, no
// second read of the file needed (docs/updater.md §14). onChunk, when
// non-nil, is called after each write with the cumulative byte count
// so far (docs/updater.md §11's downloadProgress status field).
//
// If the response declares a Content-Length exceeding maxBytes, the
// download aborts before any body bytes are read. The bound is also
// enforced while streaming, in case Content-Length was absent or
// understated (docs/updater.md §13).
func (c *Client) DownloadAssetTo(ctx context.Context, asset Asset, dest io.Writer, maxBytes int64, onChunk func(written int64)) (sha256Hex string, totalBytes int64, err error) {
	req, err := c.assetRequest(ctx, asset)
	if err != nil {
		return "", 0, err
	}

	resp, err := c.download.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %s", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	if isRateLimited(resp) {
		return "", 0, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("%w: unexpected status %d downloading %q", ErrRequestFailed, resp.StatusCode, asset.Name)
	}
	if resp.ContentLength > 0 && resp.ContentLength > maxBytes {
		return "", 0, ErrResponseTooLarge
	}

	hasher := sha256.New()
	multi := io.MultiWriter(dest, hasher)

	buf := make([]byte, downloadChunkSize)
	var total int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > maxBytes {
				return "", 0, ErrResponseTooLarge
			}
			if _, writeErr := multi.Write(buf[:n]); writeErr != nil {
				return "", 0, fmt.Errorf("%w: %s", ErrRequestFailed, writeErr)
			}
			if onChunk != nil {
				onChunk(total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", 0, fmt.Errorf("%w: %s", ErrRequestFailed, readErr)
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), total, nil
}

// isRateLimited reports whether resp looks like a GitHub rate-limit
// response (docs/updater.md §9): a 403/429 status with the rate-limit
// headers GitHub documents.
func isRateLimited(resp *http.Response) bool {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return false
	}
	return resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.StatusCode == http.StatusTooManyRequests
}

// readBounded reads at most maxBytes+1 from r, returning ErrResponseTooLarge
// if more than maxBytes was present.
func readBounded(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRequestFailed, err)
	}
	if int64(len(body)) > maxBytes {
		return nil, ErrResponseTooLarge
	}
	return body, nil
}
