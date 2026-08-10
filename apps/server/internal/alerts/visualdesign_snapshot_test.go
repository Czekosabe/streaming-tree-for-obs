package alerts

import (
	"context"
	"sync"
	"testing"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	bus "github.com/streaming-tree/server/internal/engagement"
)

// fakeVisualDesignRepo is a minimal in-memory visualdesign.Repository
// double, mirroring fakeDomainRepo's own style in manager_test.go.
type fakeVisualDesignRepo struct {
	mu      sync.Mutex
	records map[string]visualdesign.Record // keyed by ownerKind+"/"+ownerID
}

func newFakeVisualDesignRepo() *fakeVisualDesignRepo {
	return &fakeVisualDesignRepo{records: map[string]visualdesign.Record{}}
}

func vdKey(k visualdesign.OwnerKind, id string) string { return string(k) + "/" + id }

func (f *fakeVisualDesignRepo) Get(_ context.Context, k visualdesign.OwnerKind, id string) (visualdesign.Record, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.records[vdKey(k, id)]
	return rec, ok, nil
}

func (f *fakeVisualDesignRepo) Save(_ context.Context, k visualdesign.OwnerKind, id string, doc visualdesign.Document, expectedRevision int, newID func() (string, error)) (visualdesign.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.records[vdKey(k, id)]
	now := time.Now().UTC()
	if !ok {
		if expectedRevision != 0 {
			return visualdesign.Record{}, visualdesign.ErrRevisionConflict
		}
		genID, err := newID()
		if err != nil {
			return visualdesign.Record{}, err
		}
		rec := visualdesign.Record{ID: genID, OwnerKind: k, OwnerID: id, Document: doc, Revision: 1, CreatedAt: now, UpdatedAt: now}
		f.records[vdKey(k, id)] = rec
		return rec, nil
	}
	if existing.Revision != expectedRevision {
		return visualdesign.Record{}, visualdesign.ErrRevisionConflict
	}
	existing.Document = doc
	existing.Revision++
	existing.UpdatedAt = now
	f.records[vdKey(k, id)] = existing
	return existing, nil
}

func (f *fakeVisualDesignRepo) Delete(_ context.Context, k visualdesign.OwnerKind, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.records, vdKey(k, id))
	return nil
}

func fixedDesignID() (string, error) { return "design_test_fixed", nil }

// testDesignDocument returns a minimal valid Document bound to
// alert_rendered_text - available for every event type.
func testDesignDocument() visualdesign.Document {
	return visualdesign.Document{
		Version: visualdesign.CurrentVersion,
		Canvas:  visualdesign.CanvasLandscape,
		Layers: []visualdesign.Layer{{
			ID: "layer_1", Name: "Text", Kind: visualdesign.LayerText, Visible: true, Order: 0,
			Frame: visualdesign.Frame{X: 0, Y: 0, Width: 400, Height: 100}, Opacity: 1,
			Text: &visualdesign.TextProps{
				Binding: visualdesign.BindingAlertRenderedText, MissingValueBehavior: visualdesign.MissingHide,
				FontFamily: visualdesign.FontSystemUI, FontSize: 32, FontWeight: 700, LineHeight: 1.2,
				TextColor: "#FFFFFF", HorizontalAlign: visualdesign.HAlignCenter, VerticalAlign: visualdesign.VAlignMiddle,
				OutlineColor: "#000000", ShadowColor: "#000000",
			},
			EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
		}},
	}
}

// newTestManagerWithVisualDesigns is newTestManager extended with a
// fake visualdesign.Repository - a separate constructor (rather than
// changing newTestManager's own signature) so the many existing
// call sites that predate Stage 13A never need to change.
func newTestManagerWithVisualDesigns(t *testing.T, fc *fakeClock) (*Manager, *bus.Bus, *fakeVisualDesignRepo) {
	t.Helper()
	repo := newFakeDomainRepo()
	domainSvc := domain.NewService(repo, fakeDomainAccounts{}, fc.Now)
	vdRepo := newFakeVisualDesignRepo()
	vdSvc := visualdesign.NewService(vdRepo, fc.Now)
	b := bus.New(bus.Options{Now: fc.Now})
	mgr := NewManager(ManagerOptions{DomainService: domainSvc, VisualDesignService: vdSvc, Bus: b, Now: fc.Now})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	waitUntil(t, time.Second, mgr.Subscribed)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Shutdown(ctx)
		b.Shutdown()
	})
	return mgr, b, vdRepo
}

func TestSnapshotRuleWithoutDesignUsesLegacyRenderer(t *testing.T) {
	fc := newFakeClock()
	mgr, b, _ := newTestManagerWithVisualDesigns(t, fc)
	p, _ := createTestProfileAndRule(t, mgr, domain.EventFollow)

	publishFollow(t, b, fc.Now())
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p.ID)
		return err == nil && (st.Current != nil || st.QueuedCount > 0)
	})
	pr, ok := mgr.getRuntime(p.ID)
	if !ok {
		t.Fatal("runtime not found")
	}
	var inst *Instance
	pr.mu.Lock()
	if pr.current != nil {
		inst = pr.current
	} else if pr.queue.len() > 0 {
		items := pr.queue.list(1)
		inst = &items[0]
	}
	pr.mu.Unlock()
	if inst == nil {
		t.Fatal("no alert instance found")
	}
	if inst.VisualDesign != nil {
		t.Error("VisualDesign is non-nil for a rule with no saved design, want nil (legacy)")
	}
}

func TestSnapshotOpeningDraftDoesNotActivateDesignMode(t *testing.T) {
	fc := newFakeClock()
	mgr, _, vdRepo := newTestManagerWithVisualDesigns(t, fc)
	_, r := createTestProfileAndRule(t, mgr, domain.EventFollow)

	// GetVisualDesign (the "open the designer" path) must never persist
	// anything by itself.
	_, found, err := mgr.GetVisualDesign(context.Background(), r.ID)
	if err != nil {
		t.Fatalf("GetVisualDesign() error = %v", err)
	}
	if found {
		t.Fatal("GetVisualDesign() found = true before any Save, want false")
	}
	if len(vdRepo.records) != 0 {
		t.Errorf("fake repo has %d records after only a Get, want 0", len(vdRepo.records))
	}
}

func TestSnapshotSaveActivatesVisualDesignModeForNewAlerts(t *testing.T) {
	fc := newFakeClock()
	mgr, b, _ := newTestManagerWithVisualDesigns(t, fc)
	p, r := createTestProfileAndRule(t, mgr, domain.EventFollow)

	rec, err := mgr.SaveVisualDesign(context.Background(), r.ID, testDesignDocument(), 0)
	if err != nil {
		t.Fatalf("SaveVisualDesign() error = %v", err)
	}
	if rec.Revision != 1 {
		t.Fatalf("Revision = %d, want 1", rec.Revision)
	}

	publishFollow(t, b, fc.Now())
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p.ID)
		return err == nil && (st.Current != nil || st.QueuedCount > 0)
	})
	pr, _ := mgr.getRuntime(p.ID)
	pr.mu.Lock()
	var inst *Instance
	if pr.current != nil {
		inst = pr.current
	} else if pr.queue.len() > 0 {
		items := pr.queue.list(1)
		inst = &items[0]
	}
	pr.mu.Unlock()
	if inst == nil {
		t.Fatal("no alert instance found")
	}
	if inst.VisualDesign == nil {
		t.Fatal("VisualDesign is nil after saving a design, want the snapshot")
	}
	if len(inst.VisualDesign.Layers) != 1 {
		t.Errorf("snapshot has %d layers, want 1", len(inst.VisualDesign.Layers))
	}
}

func TestSnapshotTestRuleUsesTheSavedDesign(t *testing.T) {
	fc := newFakeClock()
	mgr, _, _ := newTestManagerWithVisualDesigns(t, fc)
	_, r := createTestProfileAndRule(t, mgr, domain.EventFollow)

	if _, err := mgr.SaveVisualDesign(context.Background(), r.ID, testDesignDocument(), 0); err != nil {
		t.Fatalf("SaveVisualDesign() error = %v", err)
	}
	summary, err := mgr.TestRule(context.Background(), r.ID, "")
	if err != nil {
		t.Fatalf("TestRule() error = %v", err)
	}
	if summary.AlertID == "" {
		t.Fatal("TestRule() returned an empty alert id")
	}
	// Confirm the underlying instance actually carries the design by
	// checking the queue/current snapshot directly (AlertSummary itself
	// is a management view and does not need to expose VisualDesign).
	pr, _ := mgr.getRuntime(mustProfileIDForRule(t, mgr, r.ID))
	pr.mu.Lock()
	var inst *Instance
	if pr.current != nil && pr.current.ID == summary.AlertID {
		inst = pr.current
	} else {
		for _, it := range pr.queue.list(50) {
			if it.ID == summary.AlertID {
				cp := it
				inst = &cp
				break
			}
		}
	}
	pr.mu.Unlock()
	if inst == nil {
		t.Fatal("could not find the test alert instance in the runtime")
	}
	if inst.VisualDesign == nil {
		t.Error("Test Rule's own instance has no VisualDesign, want the saved snapshot")
	}
}

func mustProfileIDForRule(t *testing.T, mgr *Manager, ruleID string) string {
	t.Helper()
	r, err := mgr.GetRule(context.Background(), ruleID)
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	return r.ProfileID
}

func TestSnapshotCurrentAlertUnaffectedByLaterSave(t *testing.T) {
	fc := newFakeClock()
	mgr, b, _ := newTestManagerWithVisualDesigns(t, fc)
	p, r := createTestProfileAndRule(t, mgr, domain.EventFollow)

	publishFollow(t, b, fc.Now())
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p.ID)
		return err == nil && st.Current != nil
	})
	pr, _ := mgr.getRuntime(p.ID)
	pr.mu.Lock()
	beforeDesign := pr.current.VisualDesign
	currentID := pr.current.ID
	pr.mu.Unlock()
	if beforeDesign != nil {
		t.Fatal("current alert already has a design before any save - test setup invalid")
	}

	if _, err := mgr.SaveVisualDesign(context.Background(), r.ID, testDesignDocument(), 0); err != nil {
		t.Fatalf("SaveVisualDesign() error = %v", err)
	}

	pr.mu.Lock()
	afterDesign := pr.current.VisualDesign
	afterID := pr.current.ID
	pr.mu.Unlock()
	if afterID != currentID {
		t.Fatalf("current alert id changed after a design save (%q -> %q), want unchanged", currentID, afterID)
	}
	if afterDesign != nil {
		t.Error("the already-current alert gained a VisualDesign after a later save - snapshot immutability violated (Part 22)")
	}
}

func TestSnapshotDeleteReturnsRuleToLegacyForNewAlertsOnly(t *testing.T) {
	fc := newFakeClock()
	mgr, b, _ := newTestManagerWithVisualDesigns(t, fc)
	p, r := createTestProfileAndRule(t, mgr, domain.EventFollow)

	if _, err := mgr.SaveVisualDesign(context.Background(), r.ID, testDesignDocument(), 0); err != nil {
		t.Fatalf("SaveVisualDesign() error = %v", err)
	}
	publishFollow(t, b, fc.Now())
	waitUntil(t, 5*time.Second, func() bool {
		st, err := mgr.ProfileStatus(p.ID)
		return err == nil && st.Current != nil
	})
	pr, _ := mgr.getRuntime(p.ID)
	pr.mu.Lock()
	if pr.current.VisualDesign == nil {
		pr.mu.Unlock()
		t.Fatal("current alert has no design before delete - test setup invalid")
	}
	currentID := pr.current.ID
	pr.mu.Unlock()

	if err := mgr.DeleteVisualDesign(context.Background(), r.ID); err != nil {
		t.Fatalf("DeleteVisualDesign() error = %v", err)
	}

	pr.mu.Lock()
	stillDesigned := pr.current.ID == currentID && pr.current.VisualDesign != nil
	pr.mu.Unlock()
	if !stillDesigned {
		t.Error("deleting the design mutated the already-current alert - snapshot immutability violated (Part 22)")
	}

	// A rule id previously never used for a follow event ensures a
	// genuinely NEW instance below, exercised via a second profile.
	summary, err := mgr.TestRule(context.Background(), r.ID, "")
	if err != nil {
		t.Fatalf("TestRule() after delete error = %v", err)
	}
	pr.mu.Lock()
	var newInst *Instance
	for _, it := range pr.queue.list(50) {
		if it.ID == summary.AlertID {
			cp := it
			newInst = &cp
			break
		}
	}
	if pr.current != nil && pr.current.ID == summary.AlertID {
		newInst = pr.current
	}
	pr.mu.Unlock()
	if newInst == nil {
		t.Fatal("could not find the new test alert instance")
	}
	if newInst.VisualDesign != nil {
		t.Error("a NEW alert created after DeleteVisualDesign still has a VisualDesign, want nil (legacy)")
	}
}
