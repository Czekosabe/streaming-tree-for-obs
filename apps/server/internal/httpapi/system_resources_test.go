package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/sysresources"
)

type fakeResourcesService struct {
	snapshot sysresources.Snapshot
}

func (f fakeResourcesService) Snapshot() sysresources.Snapshot { return f.snapshot }

func TestSystemResourcesRouteNotRegisteredWhenServiceIsNil(t *testing.T) {
	// Matches every other optional service in this router: no Resources
	// means the route simply does not exist, not a 500 from a nil
	// dereference.
	handler := NewRouter(Options{Logger: slog.Default(), StartedAt: time.Now()})

	req := httptest.NewRequest(http.MethodGet, "/api/system/resources", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when Resources is nil, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSystemResourcesReturnsTheCollectorsRealSnapshot(t *testing.T) {
	cpuPercent := 12.5
	memPercent := 48.0
	memUsed, memTotal := uint64(4_000_000_000), uint64(16_000_000_000)

	svc := fakeResourcesService{snapshot: sysresources.Snapshot{
		CPUPercent:       &cpuPercent,
		MemoryPercent:    &memPercent,
		MemoryUsedBytes:  &memUsed,
		MemoryTotalBytes: &memTotal,
		Unavailable:      []string{"disk"},
		SampledAt:        "2026-08-29T00:00:00Z",
	}}

	handler := NewRouter(Options{Logger: slog.Default(), StartedAt: time.Now(), Resources: svc})

	req := httptest.NewRequest(http.MethodGet, "/api/system/resources", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp sysresources.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response failed: %v", err)
	}

	if resp.CPUPercent == nil || *resp.CPUPercent != cpuPercent {
		t.Errorf("cpuPercent = %v, want %v", resp.CPUPercent, cpuPercent)
	}
	if resp.DiskPercent != nil {
		t.Errorf("diskPercent = %v, want nil (this fixture reported disk unavailable)", *resp.DiskPercent)
	}
	if len(resp.Unavailable) != 1 || resp.Unavailable[0] != "disk" {
		t.Errorf("unavailable = %v, want [\"disk\"]", resp.Unavailable)
	}
}

func TestSystemResourcesWrongMethodReturns405(t *testing.T) {
	handler := NewRouter(Options{
		Logger:    slog.Default(),
		StartedAt: time.Now(),
		Resources: fakeResourcesService{},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/system/resources", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Errorf("Allow header = %q, want %q", allow, http.MethodGet)
	}
}
