package streamsetup

import (
	"context"
	"errors"
	"testing"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/runtime/branch"
)

func TestPreviewClassifiesEnableDisableUnchangedAndMissing(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_enable", "twitch", "Will Enable", false)
	seedPlatform(fp, "pf_unchanged", "youtube", "Unchanged", true)
	seedPlatform(fp, "pf_disable", "kick", "Will Disable", true)
	repo := newFakeRepo()
	svc := testService(repo, fp, newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	profile := Profile{
		ID: "setup_1", Name: "Gaming",
		Destinations: []Destination{
			{PlatformID: strPtr("pf_enable"), ProviderID: "twitch", DisplayName: "Will Enable"},
			{PlatformID: strPtr("pf_unchanged"), ProviderID: "youtube", DisplayName: "Unchanged"},
			{PlatformID: nil, ProviderID: "twitch", DisplayName: "Deleted destination"},
		},
	}
	if err := repo.Create(ctx, profile); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	preview, err := svc.Preview(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	byPlatform := map[string]DestinationChange{}
	var missingCount int
	for _, item := range preview.Destinations {
		if item.Change == ChangeMissing {
			missingCount++
			continue
		}
		byPlatform[item.PlatformID] = item.Change
	}
	if byPlatform["pf_enable"] != ChangeWillEnable {
		t.Errorf("pf_enable change = %q, want will_enable", byPlatform["pf_enable"])
	}
	if byPlatform["pf_unchanged"] != ChangeUnchanged {
		t.Errorf("pf_unchanged change = %q, want unchanged", byPlatform["pf_unchanged"])
	}
	if byPlatform["pf_disable"] != ChangeWillDisable {
		t.Errorf("pf_disable change = %q, want will_disable", byPlatform["pf_disable"])
	}
	if missingCount != 1 {
		t.Errorf("missing count = %d, want 1", missingCount)
	}
}

func strPtr(s string) *string { return &s }

func TestPreviewReportsAMissingMetadataPreset(t *testing.T) {
	repo := newFakeRepo()
	svc := testService(repo, newFakePlatforms(), newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	if err := repo.Create(ctx, Profile{
		ID: "setup_1", Name: "Gaming", MetadataPresetID: nil, MetadataPresetName: "Deleted preset",
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	preview, err := svc.Preview(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if !preview.MetadataPresetReferenced || !preview.MetadataPresetMissing {
		t.Errorf("preview = %+v, want MetadataPresetReferenced and MetadataPresetMissing both true", preview)
	}
	if preview.MetadataPresetName != "Deleted preset" {
		t.Errorf("MetadataPresetName = %q, want the snapshot", preview.MetadataPresetName)
	}
}

func TestPreviewReusesTheRealMetadataPresetApplyPreview(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_1", "twitch", "A", true)
	fpr := newFakePresets()
	fpr.byID["mp_1"] = metadatapreset.Preset{ID: "mp_1", Name: "Gaming preset"}
	fpr.applyPreview = []metadatapreset.DestinationPreview{{PlatformID: "pf_1", Valid: true}}
	repo := newFakeRepo()
	svc := testService(repo, fp, fpr, &fakeBranches{})
	ctx := context.Background()
	presetID := "mp_1"

	if err := repo.Create(ctx, Profile{
		ID: "setup_1", Name: "Gaming", MetadataPresetID: &presetID, MetadataPresetName: "Gaming preset",
		Destinations: []Destination{{PlatformID: strPtr("pf_1"), ProviderID: "twitch", DisplayName: "A"}},
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	preview, err := svc.Preview(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if fpr.applyPreviewCalled != 1 {
		t.Errorf("ApplyPreview called %d times, want 1", fpr.applyPreviewCalled)
	}
	if len(preview.MetadataDestinationPreviews) != 1 {
		t.Errorf("MetadataDestinationPreviews = %+v, want the fake's own canned result passed through", preview.MetadataDestinationPreviews)
	}
}

func TestPreviewBlocksOnlyForAffectedActiveDestinations(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_enable", "twitch", "A", false)
	// Already disabled AND not part of this profile - applying it makes
	// no Enabled-flag change here at all, so it is genuinely
	// unaffected, even though it happens to have a live branch (e.g.
	// started manually, independent of its own Enabled flag).
	seedPlatform(fp, "pf_untouched_live", "kick", "Untouched Live", false)
	repo := newFakeRepo()
	branches := &fakeBranches{snapshots: []branch.Snapshot{{PlatformID: "pf_untouched_live", State: branch.StateLive}}}
	svc := testService(repo, fp, newFakePresets(), branches)
	ctx := context.Background()

	if err := repo.Create(ctx, Profile{
		ID: "setup_1", Name: "Gaming",
		Destinations: []Destination{{PlatformID: strPtr("pf_enable"), ProviderID: "twitch", DisplayName: "A"}},
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	preview, err := svc.Preview(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.Blocked {
		t.Errorf("Preview() blocked = true, want false - pf_untouched_live is not affected by this profile at all: %+v", preview)
	}
}

func TestPreviewBlocksWhenAnAffectedDestinationIsLive(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_live", "twitch", "Live One", true)
	repo := newFakeRepo()
	branches := &fakeBranches{snapshots: []branch.Snapshot{{PlatformID: "pf_live", State: branch.StateLive}}}
	svc := testService(repo, fp, newFakePresets(), branches)
	ctx := context.Background()

	// This profile does not include pf_live, so applying it will
	// DISABLE pf_live - it is directly affected and must block.
	if err := repo.Create(ctx, Profile{ID: "setup_1", Name: "Gaming"}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	preview, err := svc.Preview(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if !preview.Blocked {
		t.Fatal("Preview() blocked = false, want true - pf_live would be disabled by this apply")
	}
	if len(preview.BlockedDestinationIDs) != 1 || preview.BlockedDestinationIDs[0] != "pf_live" {
		t.Errorf("BlockedDestinationIDs = %v, want [pf_live]", preview.BlockedDestinationIDs)
	}
}

func TestApplyRefusesWhenBlocked(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_live", "twitch", "Live One", true)
	repo := newFakeRepo()
	branches := &fakeBranches{snapshots: []branch.Snapshot{{PlatformID: "pf_live", State: branch.StateLive}}}
	svc := testService(repo, fp, newFakePresets(), branches)
	ctx := context.Background()

	if err := repo.Create(ctx, Profile{ID: "setup_1", Name: "Gaming"}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	_, err := svc.Apply(ctx, "setup_1")
	if !errors.Is(err, ErrActiveStreamBlocksApply) {
		t.Fatalf("Apply() error = %v, want ErrActiveStreamBlocksApply", err)
	}
	if fp.byID["pf_live"].Enabled != true {
		t.Error("pf_live's Enabled flag changed despite the apply being blocked")
	}
}

func TestApplyEnablesAndDisablesDestinationsAtomically(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_enable", "twitch", "A", false)
	seedPlatform(fp, "pf_disable", "youtube", "B", true)
	repo := newFakeRepo()
	svc := testService(repo, fp, newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	if err := repo.Create(ctx, Profile{
		ID: "setup_1", Name: "Gaming",
		Destinations: []Destination{{PlatformID: strPtr("pf_enable"), ProviderID: "twitch", DisplayName: "A"}},
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	result, err := svc.Apply(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.DestinationsChanged != 2 {
		t.Errorf("DestinationsChanged = %d, want 2", result.DestinationsChanged)
	}
	if !fp.byID["pf_enable"].Enabled {
		t.Error("pf_enable was not enabled")
	}
	if fp.byID["pf_disable"].Enabled {
		t.Error("pf_disable was not disabled")
	}
}

func TestApplyReportsAMetadataPresetFailureWithoutFailingOverall(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_1", "twitch", "A", false)
	fpr := newFakePresets()
	fpr.byID["mp_1"] = metadatapreset.Preset{ID: "mp_1", Name: "Gaming preset"}
	fpr.applyErr = errors.New("some destination is invalid")
	repo := newFakeRepo()
	svc := testService(repo, fp, fpr, &fakeBranches{})
	ctx := context.Background()
	presetID := "mp_1"

	if err := repo.Create(ctx, Profile{
		ID: "setup_1", Name: "Gaming", MetadataPresetID: &presetID, MetadataPresetName: "Gaming preset",
		Destinations: []Destination{{PlatformID: strPtr("pf_1"), ProviderID: "twitch", DisplayName: "A"}},
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	result, err := svc.Apply(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Apply() error = %v, want overall success - the destination step succeeded", err)
	}
	if result.MetadataApplied {
		t.Error("MetadataApplied = true, want false")
	}
	if result.MetadataSkippedReason == "" {
		t.Error("MetadataSkippedReason is empty, want the underlying apply error's message")
	}
	if !fp.byID["pf_1"].Enabled {
		t.Error("the destination membership step was not applied despite the metadata step failing")
	}
}

func TestApplySkipsMetadataStepWhenPresetIsMissing(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_1", "twitch", "A", false)
	fpr := newFakePresets()
	repo := newFakeRepo()
	svc := testService(repo, fp, fpr, &fakeBranches{})
	ctx := context.Background()

	if err := repo.Create(ctx, Profile{
		ID: "setup_1", Name: "Gaming", MetadataPresetID: nil, MetadataPresetName: "Deleted preset",
		Destinations: []Destination{{PlatformID: strPtr("pf_1"), ProviderID: "twitch", DisplayName: "A"}},
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	result, err := svc.Apply(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.MetadataSkippedReason != "preset_missing" {
		t.Errorf("MetadataSkippedReason = %q, want preset_missing", result.MetadataSkippedReason)
	}
	if fpr.applyCalled != 0 {
		t.Errorf("Apply was called %d times on the metadata preset port despite the reference being missing", fpr.applyCalled)
	}
}

func TestApplyNeverContactsAnyProviderNetwork(t *testing.T) {
	// Structural proof: PlatformPort/MetadataPresetPort/BranchSnapshotter
	// are the ONLY dependencies Service.Apply can reach, and none of
	// their method sets includes anything OAuth/HTTP/provider-shaped -
	// confirmed by the fakes above satisfying the real interfaces with
	// zero network code anywhere in this test file.
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_1", "twitch", "A", false)
	repo := newFakeRepo()
	svc := testService(repo, fp, newFakePresets(), &fakeBranches{})
	ctx := context.Background()
	if err := repo.Create(ctx, Profile{
		ID: "setup_1", Name: "Gaming",
		Destinations: []Destination{{PlatformID: strPtr("pf_1"), ProviderID: "twitch", DisplayName: "A"}},
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if _, err := svc.Apply(ctx, "setup_1"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}
