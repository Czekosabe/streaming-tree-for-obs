package chatoverlay

import (
	"context"
	"errors"
	"testing"
	"time"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// fakeVisualDesignRepository is a minimal in-memory
// visualdesign.Repository double - mirrors the same fake pattern
// internal/domain/visualdesign's own service_test.go uses, reimplemented
// here since that one is unexported to its own package.
type fakeVisualDesignRepository struct {
	records map[string]visualdesign.Record
}

func newFakeVisualDesignRepository() *fakeVisualDesignRepository {
	return &fakeVisualDesignRepository{records: map[string]visualdesign.Record{}}
}

func vdKey(k visualdesign.OwnerKind, id string) string { return string(k) + "/" + id }

func (f *fakeVisualDesignRepository) Get(_ context.Context, k visualdesign.OwnerKind, id string) (visualdesign.Record, bool, error) {
	rec, ok := f.records[vdKey(k, id)]
	return rec, ok, nil
}

func (f *fakeVisualDesignRepository) Save(_ context.Context, k visualdesign.OwnerKind, id string, doc visualdesign.Document, expectedRevision int, newID func() (string, error)) (visualdesign.Record, error) {
	key := vdKey(k, id)
	existing, ok := f.records[key]
	if !ok {
		if expectedRevision != 0 {
			return visualdesign.Record{}, visualdesign.ErrRevisionConflict
		}
		genID, err := newID()
		if err != nil {
			return visualdesign.Record{}, err
		}
		rec := visualdesign.Record{ID: genID, OwnerKind: k, OwnerID: id, Document: doc, Revision: 1, CreatedAt: testTime, UpdatedAt: testTime}
		f.records[key] = rec
		return rec, nil
	}
	if existing.Revision != expectedRevision {
		return visualdesign.Record{}, visualdesign.ErrRevisionConflict
	}
	existing.Document = doc
	existing.Revision++
	existing.UpdatedAt = testTime
	f.records[key] = existing
	return existing, nil
}

func (f *fakeVisualDesignRepository) Delete(_ context.Context, k visualdesign.OwnerKind, id string) error {
	delete(f.records, vdKey(k, id))
	return nil
}

func testChatDesignDoc() visualdesign.Document {
	return visualdesign.Document{
		Version: visualdesign.CurrentVersion, Canvas: visualdesign.CanvasChatItem,
		Layers: []visualdesign.Layer{{
			ID: "layer_1", Name: "Username", Kind: visualdesign.LayerText, Visible: true, Order: 0,
			Frame: visualdesign.Frame{X: 0, Y: 0, Width: 200, Height: 32}, Opacity: 1,
			Text: &visualdesign.TextProps{
				Binding: visualdesign.BindingUsername, MissingValueBehavior: visualdesign.MissingHide,
				FontFamily: visualdesign.FontSystemUI, FontSize: 16, FontWeight: 700, LineHeight: 1.2,
				TextColor: "#FFFFFF", HorizontalAlign: visualdesign.HAlignLeft, VerticalAlign: visualdesign.VAlignMiddle,
			},
			EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
		}},
	}
}

func newTestManagerWithVisualDesigns(t *testing.T) (*Manager, *fakeResolver, *visualdesign.Service) {
	t.Helper()
	upstream := newFakeUpstream()
	resolver := newFakeResolver()
	vdSvc := visualdesign.NewService(newFakeVisualDesignRepository(), func() time.Time { return testTime })
	m := NewManager(upstream, resolver, vdSvc, nil)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m, resolver, vdSvc
}

func TestManagerGetVisualDesignUnavailableWithNilService(t *testing.T) {
	m, _, _ := newTestManager(t) // newTestManager wires visualDesignSvc=nil
	_, _, err := m.GetVisualDesign(context.Background(), "ov_1")
	if !errors.Is(err, ErrVisualDesignUnavailable) {
		t.Fatalf("GetVisualDesign() error = %v, want ErrVisualDesignUnavailable", err)
	}
}

func TestManagerSaveVisualDesignPersistsAndNotifiesPresentation(t *testing.T) {
	m, resolver, _ := newTestManagerWithVisualDesigns(t)
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))

	p, err := m.EnsureOverlay(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("EnsureOverlay() error = %v", err)
	}
	sub, _, _ := p.Subscribe(0)
	defer sub.Cancel()
	waitRevision(time.Second, sub.Revisions()) // initial reset from EnsureOverlay

	rec, err := m.SaveVisualDesign(context.Background(), "ov_1", testChatDesignDoc(), 0)
	if err != nil {
		t.Fatalf("SaveVisualDesign() error = %v", err)
	}
	if rec.Revision != 1 {
		t.Errorf("Revision = %d, want 1", rec.Revision)
	}

	_, found, err := m.GetVisualDesign(context.Background(), "ov_1")
	if err != nil || !found {
		t.Fatalf("GetVisualDesign() found = %v, err = %v", found, err)
	}

	// SaveVisualDesign triggers Rebuild (a reset) followed by
	// NotifyPresentationChanged - both must reach the subscriber.
	first, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || first.Operation != OpReset {
		t.Fatalf("expected a reset revision after save, got %+v, ok=%v", first, ok)
	}
	second, ok := waitRevision(time.Second, sub.Revisions())
	if !ok || second.Operation != OpPresentationChanged {
		t.Fatalf("expected a presentation_changed revision after save, got %+v, ok=%v", second, ok)
	}
}

func TestManagerSaveVisualDesignRejectsAlertOnlyBinding(t *testing.T) {
	m, resolver, _ := newTestManagerWithVisualDesigns(t)
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))

	doc := testChatDesignDoc()
	doc.Layers[0].Text.Binding = visualdesign.BindingAlertRenderedText
	_, err := m.SaveVisualDesign(context.Background(), "ov_1", doc, 0)
	if !errors.Is(err, visualdesign.ErrValidation) {
		t.Fatalf("SaveVisualDesign() with alert_rendered_text error = %v, want ErrValidation", err)
	}
}

func TestManagerSaveVisualDesignReturnsConflictOnStaleRevision(t *testing.T) {
	m, resolver, _ := newTestManagerWithVisualDesigns(t)
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))

	if _, err := m.SaveVisualDesign(context.Background(), "ov_1", testChatDesignDoc(), 0); err != nil {
		t.Fatalf("first SaveVisualDesign() error = %v", err)
	}
	_, err := m.SaveVisualDesign(context.Background(), "ov_1", testChatDesignDoc(), 0)
	if !errors.Is(err, visualdesign.ErrRevisionConflict) {
		t.Fatalf("stale-revision SaveVisualDesign() error = %v, want ErrRevisionConflict", err)
	}
}

func TestManagerDeleteVisualDesignReturnsToLegacy(t *testing.T) {
	m, resolver, _ := newTestManagerWithVisualDesigns(t)
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))

	if _, err := m.SaveVisualDesign(context.Background(), "ov_1", testChatDesignDoc(), 0); err != nil {
		t.Fatalf("SaveVisualDesign() error = %v", err)
	}
	if err := m.DeleteVisualDesign(context.Background(), "ov_1"); err != nil {
		t.Fatalf("DeleteVisualDesign() error = %v", err)
	}
	_, found, err := m.GetVisualDesign(context.Background(), "ov_1")
	if err != nil || found {
		t.Fatalf("GetVisualDesign() after delete found = %v, err = %v, want found=false", found, err)
	}
	// Idempotent.
	if err := m.DeleteVisualDesign(context.Background(), "ov_1"); err != nil {
		t.Fatalf("second DeleteVisualDesign() error = %v, want nil (idempotent)", err)
	}
}

func TestManagerRemoveCascadesVisualDesignDeletion(t *testing.T) {
	m, resolver, _ := newTestManagerWithVisualDesigns(t)
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))

	if _, err := m.EnsureOverlay(context.Background(), "ov_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SaveVisualDesign(context.Background(), "ov_1", testChatDesignDoc(), 0); err != nil {
		t.Fatalf("SaveVisualDesign() error = %v", err)
	}

	m.Remove(context.Background(), "ov_1")

	_, found, err := m.GetVisualDesign(context.Background(), "ov_1")
	if err != nil || found {
		t.Fatalf("GetVisualDesign() after Remove found = %v, err = %v, want found=false (cascade delete)", found, err)
	}
}

func TestManagerRemoveDoesNotAffectAnotherOverlaysDesign(t *testing.T) {
	m, resolver, _ := newTestManagerWithVisualDesigns(t)
	resolver.set("ov_1", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_1" }))
	resolver.set("ov_2", testSettings(func(p *chatoverlaydomain.Profile) { p.ID = "ov_2" }))

	if _, err := m.SaveVisualDesign(context.Background(), "ov_1", testChatDesignDoc(), 0); err != nil {
		t.Fatalf("SaveVisualDesign(ov_1) error = %v", err)
	}
	if _, err := m.SaveVisualDesign(context.Background(), "ov_2", testChatDesignDoc(), 0); err != nil {
		t.Fatalf("SaveVisualDesign(ov_2) error = %v", err)
	}

	m.Remove(context.Background(), "ov_1")

	_, found, err := m.GetVisualDesign(context.Background(), "ov_2")
	if err != nil || !found {
		t.Fatalf("GetVisualDesign(ov_2) after removing ov_1 found = %v, err = %v, want found=true", found, err)
	}
}
