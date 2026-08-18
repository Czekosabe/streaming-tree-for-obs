package updater

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newClient(server.URL, "0.1.0")
	return server, client
}

func sampleWireRelease() wireRelease {
	return wireRelease{
		ID: 1, TagName: "v0.2.0", Name: "0.2.0",
		Draft: false, Prerelease: false, Body: "release notes",
		PublishedAt: time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC),
		Assets: []wireAsset{
			{
				ID: 10, Name: "StreamingTreeForOBS-Setup-0.2.0.exe", Size: 100,
				BrowserDownloadURL: "https://example.invalid/dl/setup.exe",
				URL:                "https://example.invalid/api/assets/10",
				ContentType:        "application/octet-stream",
				Digest:             "sha256:" + strings.Repeat("a", 64),
			},
			{
				ID: 11, Name: manifestAssetName, Size: 200,
				BrowserDownloadURL: "https://example.invalid/dl/manifest.json",
				URL:                "https://example.invalid/api/assets/11",
				ContentType:        "application/json",
			},
		},
	}
}

func TestFetchLatestReleaseSuccess(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/Czekosabe/streaming-tree-for-obs/releases/latest" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != acceptHeader {
			t.Errorf("Accept header = %q, want %q", got, acceptHeader)
		}
		if got := r.Header.Get(apiVersionHeader); got != apiVersionValue {
			t.Errorf("%s header = %q, want %q", apiVersionHeader, got, apiVersionValue)
		}
		if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "StreamingTreeForOBS/0.1.0") {
			t.Errorf("User-Agent = %q, want it to start with StreamingTreeForOBS/0.1.0", got)
		}
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(sampleWireRelease())
	})

	result, err := client.FetchLatestRelease(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchLatestRelease() error = %v", err)
	}
	if result.NotModified {
		t.Fatal("NotModified = true, want false")
	}
	if result.ETag != `"abc123"` {
		t.Fatalf("ETag = %q, want %q", result.ETag, `"abc123"`)
	}
	if result.Release == nil {
		t.Fatal("Release = nil")
	}
	if result.Release.TagName != "v0.2.0" {
		t.Fatalf("TagName = %q, want v0.2.0", result.Release.TagName)
	}
	if len(result.Release.Assets) != 2 {
		t.Fatalf("Assets = %d, want 2", len(result.Release.Assets))
	}
}

func TestFetchLatestReleasePreservesDraftPrerelease(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		wr := sampleWireRelease()
		wr.Draft = true
		wr.Prerelease = true
		_ = json.NewEncoder(w).Encode(wr)
	})

	result, err := client.FetchLatestRelease(context.Background(), "")
	if err != nil {
		t.Fatalf("FetchLatestRelease() error = %v", err)
	}
	if !result.Release.Draft || !result.Release.Prerelease {
		t.Fatal("draft/prerelease flags not preserved - caller must be able to re-check them")
	}
}

func TestFetchLatestReleaseETagRoundTrip(t *testing.T) {
	var sawIfNoneMatch string
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		sawIfNoneMatch = r.Header.Get("If-None-Match")
		if sawIfNoneMatch == `"abc123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"abc123"`)
		_ = json.NewEncoder(w).Encode(sampleWireRelease())
	})

	result, err := client.FetchLatestRelease(context.Background(), `"abc123"`)
	if err != nil {
		t.Fatalf("FetchLatestRelease() error = %v", err)
	}
	if sawIfNoneMatch != `"abc123"` {
		t.Fatalf("server did not see If-None-Match, got %q", sawIfNoneMatch)
	}
	if !result.NotModified {
		t.Fatal("NotModified = false, want true (304 is a normal success outcome)")
	}
	if result.Release != nil {
		t.Fatal("Release should be nil on a 304 response")
	}
}

func TestFetchLatestReleaseRateLimited(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := client.FetchLatestRelease(context.Background(), "")
	if err != ErrRateLimited {
		t.Fatalf("error = %v, want ErrRateLimited", err)
	}
}

func TestFetchLatestReleaseTooManyRequests(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := client.FetchLatestRelease(context.Background(), "")
	if err != ErrRateLimited {
		t.Fatalf("error = %v, want ErrRateLimited", err)
	}
}

func TestFetchLatestReleaseMalformedJSON(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not json"))
	})

	if _, err := client.FetchLatestRelease(context.Background(), ""); err == nil {
		t.Fatal("FetchLatestRelease() accepted malformed JSON, want an error")
	}
}

func TestFetchLatestReleaseUnexpectedStatus(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := client.FetchLatestRelease(context.Background(), ""); err == nil {
		t.Fatal("FetchLatestRelease() accepted a 500 response, want an error")
	}
}

func TestFetchLatestReleaseOversizedResponse(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		buf := make([]byte, maxMetadataResponseBytes+1024)
		_, _ = w.Write(buf)
	})

	_, err := client.FetchLatestRelease(context.Background(), "")
	if err != ErrResponseTooLarge {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
}

func TestAssetByNameExactMatch(t *testing.T) {
	release := sampleWireRelease().toRelease()

	a, err := release.AssetByName("StreamingTreeForOBS-Setup-0.2.0.exe")
	if err != nil {
		t.Fatalf("AssetByName() error = %v", err)
	}
	if a.ID != 10 {
		t.Fatalf("AssetByName() ID = %d, want 10", a.ID)
	}
}

func TestAssetByNameNoSubstringMatch(t *testing.T) {
	release := sampleWireRelease().toRelease()

	if _, err := release.AssetByName("Setup-0.2.0"); err == nil {
		t.Fatal("AssetByName() matched a substring, want ErrAssetNotFound")
	}
}

func TestAssetByNameCaseSensitive(t *testing.T) {
	release := sampleWireRelease().toRelease()

	if _, err := release.AssetByName("streamingtreeforobs-setup-0.2.0.exe"); err == nil {
		t.Fatal("AssetByName() matched case-insensitively, want ErrAssetNotFound")
	}
}

func TestAssetByNameAmbiguous(t *testing.T) {
	release := sampleWireRelease().toRelease()
	release.Assets = append(release.Assets, release.Assets[0])

	if _, err := release.AssetByName(release.Assets[0].Name); !errors.Is(err, ErrAssetAmbiguous) {
		t.Fatalf("error = %v, want it to wrap ErrAssetAmbiguous", err)
	}
}

func TestManifestAssetFound(t *testing.T) {
	release := sampleWireRelease().toRelease()
	a, err := release.ManifestAsset()
	if err != nil {
		t.Fatalf("ManifestAsset() error = %v", err)
	}
	if a.Name != manifestAssetName {
		t.Fatalf("ManifestAsset() name = %q, want %q", a.Name, manifestAssetName)
	}
}

func TestFetchManifestDownloadsAndBounds(t *testing.T) {
	payload := []byte(`{"format":"streaming-tree-release"}`)
	server, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/octet-stream" {
			t.Errorf("Accept = %q, want application/octet-stream", got)
		}
		_, _ = w.Write(payload)
	})

	asset := Asset{Name: manifestAssetName, APIURL: server.URL + "/dl/manifest.json"}
	got, err := client.FetchManifest(context.Background(), asset)
	if err != nil {
		t.Fatalf("FetchManifest() error = %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("FetchManifest() = %q, want %q", got, payload)
	}
}

func TestFetchManifestOversized(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, maxManifestResponseBytes+1024)
		_, _ = w.Write(buf)
	})

	_, err := client.FetchManifest(context.Background(), Asset{APIURL: client.baseURL})
	if err != ErrResponseTooLarge {
		t.Fatalf("error = %v, want ErrResponseTooLarge", err)
	}
}

func TestFetchManifestRateLimited(t *testing.T) {
	_, client := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := client.FetchManifest(context.Background(), Asset{APIURL: client.baseURL})
	if err != ErrRateLimited {
		t.Fatalf("error = %v, want ErrRateLimited", err)
	}
}

func TestCrossCheckDigestAgrees(t *testing.T) {
	sha := strings.Repeat("a", 64)
	asset := Asset{Digest: "sha256:" + sha}
	if err := CrossCheckDigest(asset, sha); err != nil {
		t.Fatalf("CrossCheckDigest() error = %v, want nil", err)
	}
}

func TestCrossCheckDigestCaseInsensitiveHexCompare(t *testing.T) {
	asset := Asset{Digest: "sha256:" + strings.Repeat("A", 64)}
	if err := CrossCheckDigest(asset, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("CrossCheckDigest() error = %v, want nil", err)
	}
}

func TestCrossCheckDigestMismatch(t *testing.T) {
	asset := Asset{Digest: "sha256:" + strings.Repeat("a", 64)}
	if err := CrossCheckDigest(asset, strings.Repeat("b", 64)); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("error = %v, want it to wrap ErrDigestMismatch", err)
	}
}

func TestCrossCheckDigestAbsentIsFine(t *testing.T) {
	asset := Asset{Digest: ""}
	if err := CrossCheckDigest(asset, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("CrossCheckDigest() error = %v, want nil for an absent digest", err)
	}
}

func TestCrossCheckDigestUnknownAlgorithmIgnored(t *testing.T) {
	asset := Asset{Digest: "md5:deadbeef"}
	if err := CrossCheckDigest(asset, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("CrossCheckDigest() error = %v, want nil for an unrecognized digest prefix", err)
	}
}
