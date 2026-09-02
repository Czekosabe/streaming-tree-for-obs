package preflight

import (
	"context"
	"errors"
	"testing"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
	"github.com/streaming-tree/server/internal/runtime/branch"
)

// --- fakes -----------------------------------------------------------

type fakeBranches struct {
	blockers  map[string][]string
	blockErr  error
	snapshots []branch.Snapshot
}

func newFakeBranches() *fakeBranches {
	return &fakeBranches{blockers: map[string][]string{}}
}

func (f *fakeBranches) EvaluateReadiness(_ context.Context, platformID string) ([]string, error) {
	if f.blockErr != nil {
		return nil, f.blockErr
	}
	return f.blockers[platformID], nil
}

func (f *fakeBranches) Snapshot(context.Context) ([]branch.Snapshot, error) {
	return f.snapshots, nil
}

type fakePlatforms struct{ rows []platform.Platform }

func (f *fakePlatforms) List(context.Context) ([]platform.Platform, error) { return f.rows, nil }

func seedPlatform(id, provider, name string, enabled bool) platform.Platform {
	return platform.Platform{ID: id, ProviderID: platform.ProviderID(provider), DisplayName: name, Enabled: enabled}
}

type fakeAccounts struct {
	links    map[string]account.Link
	accounts map[string]account.Account
}

func newFakeAccounts() *fakeAccounts {
	return &fakeAccounts{links: map[string]account.Link{}, accounts: map[string]account.Account{}}
}

func (f *fakeAccounts) GetLink(_ context.Context, platformID string) (account.Link, bool, error) {
	l, ok := f.links[platformID]
	return l, ok, nil
}

func (f *fakeAccounts) GetAccount(_ context.Context, id string) (account.Account, error) {
	a, ok := f.accounts[id]
	if !ok {
		return account.Account{}, errors.New("account not found")
	}
	return a, nil
}

type fakeSetups struct {
	previews map[string]streamsetup.Preview
	err      error
}

func newFakeSetups() *fakeSetups {
	return &fakeSetups{previews: map[string]streamsetup.Preview{}}
}

func (f *fakeSetups) Preview(_ context.Context, profileID string) (streamsetup.Preview, error) {
	if f.err != nil {
		return streamsetup.Preview{}, f.err
	}
	p, ok := f.previews[profileID]
	if !ok {
		return streamsetup.Preview{}, streamsetup.ErrNotFound
	}
	return p, nil
}

func testService(branches *fakeBranches, platforms *fakePlatforms, accounts *fakeAccounts, setups *fakeSetups) *Service {
	return NewService(branches, platforms, accounts, setups)
}

// --- tests -------------------------------------------------------------

func TestEvaluateWithNoProfileUsesEveryEnabledDestination(t *testing.T) {
	platforms := &fakePlatforms{rows: []platform.Platform{
		seedPlatform("pf_1", "twitch", "Enabled One", true),
		seedPlatform("pf_2", "youtube", "Disabled One", false),
	}}
	branches := newFakeBranches()
	svc := testService(branches, platforms, newFakeAccounts(), newFakeSetups())

	report, err := svc.Evaluate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(report.Destinations) != 1 || report.Destinations[0].PlatformID != "pf_1" {
		t.Fatalf("Destinations = %+v, want exactly pf_1", report.Destinations)
	}
	if report.Status != StatusReady {
		t.Errorf("Status = %q, want ready", report.Status)
	}
	if report.SelectedProfileID != nil {
		t.Errorf("SelectedProfileID = %v, want nil", report.SelectedProfileID)
	}
}

func TestEvaluateWithNoBlockersOrWarningsIsReady(t *testing.T) {
	platforms := &fakePlatforms{rows: []platform.Platform{seedPlatform("pf_1", "twitch", "A", true)}}
	svc := testService(newFakeBranches(), platforms, newFakeAccounts(), newFakeSetups())

	report, err := svc.Evaluate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if report.Status != StatusReady {
		t.Errorf("Status = %q, want ready", report.Status)
	}
	if len(report.Findings) != 0 {
		t.Errorf("Findings = %+v, want none", report.Findings)
	}
}

func TestEvaluateReportsABlockerAsNotReadyWithAnAction(t *testing.T) {
	platforms := &fakePlatforms{rows: []platform.Platform{seedPlatform("pf_1", "twitch", "A", true)}}
	branches := newFakeBranches()
	branches.blockers["pf_1"] = []string{branch.BlockerStreamKeyMissing}
	svc := testService(branches, platforms, newFakeAccounts(), newFakeSetups())

	report, err := svc.Evaluate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if report.Status != StatusNotReady {
		t.Fatalf("Status = %q, want not_ready", report.Status)
	}
	if len(report.Findings) != 1 || report.Findings[0].Code != branch.BlockerStreamKeyMissing {
		t.Fatalf("Findings = %+v, want exactly stream_key_missing", report.Findings)
	}
	if report.Findings[0].Action == nil || report.Findings[0].Action.Code != ActionAddStreamKey {
		t.Errorf("Action = %+v, want add_stream_key", report.Findings[0].Action)
	}
}

func TestEvaluateIngestNotReceivingHasNoAction(t *testing.T) {
	platforms := &fakePlatforms{rows: []platform.Platform{seedPlatform("pf_1", "twitch", "A", true)}}
	branches := newFakeBranches()
	branches.blockers["pf_1"] = []string{branch.BlockerIngestNotReceiving}
	svc := testService(branches, platforms, newFakeAccounts(), newFakeSetups())

	report, err := svc.Evaluate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if report.Findings[0].Action != nil {
		t.Errorf("Action = %+v, want nil (the operator must start OBS themselves)", report.Findings[0].Action)
	}
}

func TestEvaluateAccountReconnectRequiredIsAWarningNeverABlocker(t *testing.T) {
	platforms := &fakePlatforms{rows: []platform.Platform{seedPlatform("pf_1", "twitch", "A", true)}}
	accounts := newFakeAccounts()
	accounts.links["pf_1"] = account.Link{PlatformID: "pf_1", AccountID: "acct_1"}
	accounts.accounts["acct_1"] = account.Account{ID: "acct_1", Status: account.StatusReconnectRequired}
	svc := testService(newFakeBranches(), platforms, accounts, newFakeSetups())

	report, err := svc.Evaluate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if report.Status != StatusReadyWithWarnings {
		t.Fatalf("Status = %q, want ready_with_warnings (never not_ready for an account issue)", report.Status)
	}
	if len(report.Findings) != 1 || report.Findings[0].Severity != SeverityWarning || report.Findings[0].Code != "account_reconnect_required" {
		t.Fatalf("Findings = %+v, want exactly one account_reconnect_required warning", report.Findings)
	}
}

func TestEvaluateInvalidLocalMetadataIsAWarning(t *testing.T) {
	platforms := &fakePlatforms{rows: []platform.Platform{
		// A case-insensitive duplicate tag is a real ValidateMetadata
		// rejection (RuleDuplicate) - Twitch supports Tags, so this is
		// a deterministic, provider-agnostic way to force an invalid
		// stored metadata state without depending on any enum option.
		{ID: "pf_1", ProviderID: platform.ProviderTwitch, DisplayName: "A", Enabled: true,
			Metadata: platform.Metadata{Tags: []string{"Go", "go"}}},
	}}
	svc := testService(newFakeBranches(), platforms, newFakeAccounts(), newFakeSetups())

	report, err := svc.Evaluate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Code == "metadata_invalid" && f.Severity == SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("Findings = %+v, want a metadata_invalid warning for a duplicate tag", report.Findings)
	}
	if report.Status == StatusNotReady {
		t.Error("Status = not_ready, want ready_with_warnings - metadata validity never blocks Start")
	}
}

func TestEvaluateWithAProfileUsesOnlyItsDestinations(t *testing.T) {
	platforms := &fakePlatforms{rows: []platform.Platform{
		seedPlatform("pf_1", "twitch", "In profile", true),
		seedPlatform("pf_2", "youtube", "Not in profile", true),
	}}
	setups := newFakeSetups()
	setups.previews["setup_1"] = streamsetup.Preview{
		Destinations: []streamsetup.DestinationPreviewItem{
			{PlatformID: "pf_1", ProviderID: "twitch", DisplayName: "In profile", Change: streamsetup.ChangeUnchanged},
		},
	}
	svc := testService(newFakeBranches(), platforms, newFakeAccounts(), setups)

	profileID := "setup_1"
	report, err := svc.Evaluate(context.Background(), &profileID)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(report.Destinations) != 1 || report.Destinations[0].PlatformID != "pf_1" {
		t.Fatalf("Destinations = %+v, want exactly pf_1", report.Destinations)
	}
	if report.SelectedProfileID == nil || *report.SelectedProfileID != "setup_1" {
		t.Errorf("SelectedProfileID = %v, want setup_1", report.SelectedProfileID)
	}
}

func TestEvaluateReportsAMissingProfileDestinationAndPreset(t *testing.T) {
	platforms := &fakePlatforms{rows: []platform.Platform{}}
	setups := newFakeSetups()
	setups.previews["setup_1"] = streamsetup.Preview{
		Destinations: []streamsetup.DestinationPreviewItem{
			{ProviderID: "twitch", DisplayName: "Deleted", Change: streamsetup.ChangeMissing},
		},
		MetadataPresetMissing: true,
	}
	svc := testService(newFakeBranches(), platforms, newFakeAccounts(), setups)

	profileID := "setup_1"
	report, err := svc.Evaluate(context.Background(), &profileID)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(report.Destinations) != 0 {
		t.Errorf("Destinations = %+v, want none (the only referenced one is missing)", report.Destinations)
	}
	var gotDestMissing, gotPresetMissing bool
	for _, f := range report.Findings {
		if f.Code == "setup_destination_missing" {
			gotDestMissing = true
		}
		if f.Code == "setup_metadata_preset_missing" {
			gotPresetMissing = true
		}
		if f.Severity != SeverityWarning {
			t.Errorf("a profile-integrity finding must be a warning, got %+v", f)
		}
	}
	if !gotDestMissing || !gotPresetMissing {
		t.Fatalf("Findings = %+v, want both setup_destination_missing and setup_metadata_preset_missing", report.Findings)
	}
	if report.Status != StatusReadyWithWarnings {
		t.Errorf("Status = %q, want ready_with_warnings", report.Status)
	}
}

func TestEvaluateWithAnUnknownProfileReturnsTheUnderlyingError(t *testing.T) {
	svc := testService(newFakeBranches(), &fakePlatforms{}, newFakeAccounts(), newFakeSetups())
	profileID := "setup_missing"
	_, err := svc.Evaluate(context.Background(), &profileID)
	if !errors.Is(err, streamsetup.ErrNotFound) {
		t.Fatalf("Evaluate() error = %v, want ErrNotFound", err)
	}
}

func TestEvaluateReportsStreamingActive(t *testing.T) {
	platforms := &fakePlatforms{rows: []platform.Platform{seedPlatform("pf_1", "twitch", "A", true)}}
	branches := newFakeBranches()
	branches.snapshots = []branch.Snapshot{{PlatformID: "pf_1", State: branch.StateLive}}
	svc := testService(branches, platforms, newFakeAccounts(), newFakeSetups())

	report, err := svc.Evaluate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !report.StreamingActive {
		t.Error("StreamingActive = false, want true for a live branch")
	}
}
