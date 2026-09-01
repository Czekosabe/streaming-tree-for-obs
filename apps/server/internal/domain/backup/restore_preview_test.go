package backup

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/platform"
)

func newTestServiceForRestorePreview(t *testing.T) *Service {
	t.Helper()
	staging, err := NewFileStaging(filepath.Join(t.TempDir(), "staging"), time.Minute)
	if err != nil {
		t.Fatalf("NewFileStaging() error = %v", err)
	}
	return NewService(Sources{}, memBlobSource{}, memBlobSource{}, staging, "0.1.0-test", "windows")
}

func TestRestorePreviewSummarizesWithoutMutating(t *testing.T) {
	cfg := Config{
		FormatVersion: FormatVersion,
		Platforms: []PlatformExport{
			{Platform: platform.Platform{ID: "pf_1"}}, {Platform: platform.Platform{ID: "pf_2"}},
		},
		ConnectedAccounts: []ConnectedAccountExport{{Account: account.Account{ID: "acct_1"}}},
		AlertProfiles: []AlertProfileExport{
			{Profile: alerts.Profile{ID: "alp_1"}, Rules: []alerts.Rule{{ID: "alr_1"}, {ID: "alr_2"}}},
		},
	}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	svc := newTestServiceForRestorePreview(t)
	preview, err := svc.RestorePreview(context.Background(), data)
	if err != nil {
		t.Fatalf("RestorePreview() error = %v", err)
	}

	if preview.Token == "" {
		t.Error("Token is empty")
	}
	if preview.Counts.Platforms != 2 {
		t.Errorf("Counts.Platforms = %d, want 2", preview.Counts.Platforms)
	}
	if preview.Counts.ConnectedAccounts != 1 {
		t.Errorf("Counts.ConnectedAccounts = %d, want 1", preview.Counts.ConnectedAccounts)
	}
	if preview.Counts.AlertProfiles != 1 || preview.Counts.AlertRules != 2 {
		t.Errorf("AlertProfiles/AlertRules = %d/%d, want 1/2", preview.Counts.AlertProfiles, preview.Counts.AlertRules)
	}
	if preview.ConnectedAccountsRequireReconnect != 1 {
		t.Errorf("ConnectedAccountsRequireReconnect = %d, want 1", preview.ConnectedAccountsRequireReconnect)
	}
	if preview.DestinationsNeedStreamKey != 2 {
		t.Errorf("DestinationsNeedStreamKey = %d, want 2", preview.DestinationsNeedStreamKey)
	}
	if preview.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt is already in the past")
	}
}

func TestRestorePreviewStagesRawBytesForLaterCommit(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	staging, err := NewFileStaging(filepath.Join(t.TempDir(), "staging"), time.Minute)
	if err != nil {
		t.Fatalf("NewFileStaging() error = %v", err)
	}
	svc := NewService(Sources{}, memBlobSource{}, memBlobSource{}, staging, "0.1.0-test", "windows")

	preview, err := svc.RestorePreview(context.Background(), data)
	if err != nil {
		t.Fatalf("RestorePreview() error = %v", err)
	}

	staged, err := staging.Get(preview.Token)
	if err != nil {
		t.Fatalf("staging.Get(token) error = %v", err)
	}
	if len(staged) != len(data) {
		t.Errorf("staged bytes length = %d, want %d (the original upload, byte-identical)", len(staged), len(data))
	}
}

func TestRestorePreviewRejectsInvalidPackageAndStagesNothing(t *testing.T) {
	svc := newTestServiceForRestorePreview(t)
	if _, err := svc.RestorePreview(context.Background(), []byte("not a zip")); err == nil {
		t.Fatal("RestorePreview() succeeded for garbage input, want an error")
	}
}

func TestCancelPreviewRemovesStagedBytes(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	svc := newTestServiceForRestorePreview(t)
	preview, err := svc.RestorePreview(context.Background(), data)
	if err != nil {
		t.Fatalf("RestorePreview() error = %v", err)
	}

	svc.CancelPreview(preview.Token)

	if _, err := svc.staging.Get(preview.Token); err == nil {
		t.Fatal("staged bytes still readable after CancelPreview")
	}
}
