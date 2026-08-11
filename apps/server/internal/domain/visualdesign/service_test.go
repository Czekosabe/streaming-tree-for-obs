package visualdesign

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepository is a deterministic in-memory Repository double for
// Service's own tests - mirrors the pattern used by every other domain
// package's own service_test.go in this project (a fake, not a mock
// framework).
type fakeRepository struct {
	records map[string]Record // keyed by "ownerKind/ownerID"
	nextInt int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{records: map[string]Record{}}
}

func key(k OwnerKind, id string) string { return string(k) + "/" + id }

func (f *fakeRepository) Get(_ context.Context, k OwnerKind, id string) (Record, bool, error) {
	rec, ok := f.records[key(k, id)]
	return rec, ok, nil
}

func (f *fakeRepository) Save(_ context.Context, k OwnerKind, id string, doc Document, expectedRevision int, newID func() (string, error)) (Record, error) {
	existing, ok := f.records[key(k, id)]
	now := time.Unix(int64(f.nextInt), 0).UTC()
	f.nextInt++
	if !ok {
		if expectedRevision != 0 {
			return Record{}, ErrRevisionConflict
		}
		genID, err := newID()
		if err != nil {
			return Record{}, err
		}
		rec := Record{ID: genID, OwnerKind: k, OwnerID: id, Document: doc, Revision: 1, CreatedAt: now, UpdatedAt: now}
		f.records[key(k, id)] = rec
		return rec, nil
	}
	if existing.Revision != expectedRevision {
		return Record{}, ErrRevisionConflict
	}
	existing.Document = doc
	existing.Revision++
	existing.UpdatedAt = now
	f.records[key(k, id)] = existing
	return existing, nil
}

func (f *fakeRepository) Delete(_ context.Context, k OwnerKind, id string) error {
	delete(f.records, key(k, id))
	return nil
}

func fixedID() (string, error) { return "design_fixed", nil }

func newTestService() (*Service, *fakeRepository) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)
	svc.newID = fixedID
	return svc, repo
}

func TestServiceGetReturnsNotFoundForAnUnsavedOwner(t *testing.T) {
	svc, _ := newTestService()
	_, found, err := svc.Get(context.Background(), OwnerKindAlertRule, "alrule_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Fatal("Get() found = true, want false for an owner with no saved design")
	}
}

func TestServiceRejectsAnUnacceptedOwnerKind(t *testing.T) {
	// "chat_overlay" was this test's own example of an unaccepted owner
	// kind in Stage 13A; Stage 13B made it a genuinely accepted one (see
	// TestServiceAcceptsChatOverlayOwnerKind below), so this test now
	// uses a still-genuinely-unknown owner kind instead.
	svc, _ := newTestService()
	_, _, err := svc.Get(context.Background(), OwnerKind("widget"), "widget_1")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("Get() error = %v, want ErrValidation for an unaccepted owner kind", err)
	}
}

func TestServiceAcceptsChatOverlayOwnerKind(t *testing.T) {
	svc, _ := newTestService()
	rec, err := svc.Save(context.Background(), OwnerKindChatOverlay, "co_1", validDoc(), 0)
	if err != nil {
		t.Fatalf("Save() error = %v, want chat_overlay to be an accepted owner kind", err)
	}
	if rec.OwnerKind != OwnerKindChatOverlay {
		t.Errorf("OwnerKind = %q, want %q", rec.OwnerKind, OwnerKindChatOverlay)
	}
}

func TestServiceSaveCreatesAtRevisionOne(t *testing.T) {
	svc, _ := newTestService()
	rec, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", validDoc(), 0)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if rec.Revision != 1 {
		t.Errorf("Revision = %d, want 1", rec.Revision)
	}
	if rec.ID != "design_fixed" {
		t.Errorf("ID = %q, want design_fixed", rec.ID)
	}
}

func TestServiceSaveRejectsCreateWithNonZeroExpectedRevision(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", validDoc(), 3)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("Save() error = %v, want ErrRevisionConflict", err)
	}
}

func TestServiceSaveIncrementsRevisionOnReplace(t *testing.T) {
	svc, _ := newTestService()
	first, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", validDoc(), 0)
	if err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	second, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", validDoc(), first.Revision)
	if err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	if second.Revision != first.Revision+1 {
		t.Errorf("second.Revision = %d, want %d", second.Revision, first.Revision+1)
	}
}

func TestServiceSaveReturnsConflictOnStaleRevision(t *testing.T) {
	svc, _ := newTestService()
	first, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", validDoc(), 0)
	if err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if _, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", validDoc(), first.Revision); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	// first.Revision is now stale (the second save already advanced it).
	_, err = svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", validDoc(), first.Revision)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale Save() error = %v, want ErrRevisionConflict", err)
	}
}

func TestServiceSaveRejectsAnInvalidDocument(t *testing.T) {
	svc, _ := newTestService()
	invalid := validDoc()
	invalid.Version = 42
	_, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", invalid, 0)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("Save() error = %v, want ErrValidation", err)
	}
}

func TestServiceSaveNormalizesLayerOrderToADenseSequence(t *testing.T) {
	svc, repo := newTestService()
	doc := validDoc()
	doc.Layers[0].Order = 50
	doc.Layers[1].Order = 5
	if _, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", doc, 0); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	saved := repo.records[key(OwnerKindAlertRule, "alrule_1")].Document
	// layer with Order=5 (doc.Layers[1]) sorts first -> normalized to 0;
	// layer with Order=50 (doc.Layers[0]) sorts second -> normalized to 1.
	var orderByID = map[string]int{}
	for _, l := range saved.Layers {
		orderByID[l.ID] = l.Order
	}
	if orderByID["layer_2"] != 0 || orderByID["layer_1"] != 1 {
		t.Errorf("normalized order = %+v, want layer_2=0, layer_1=1", orderByID)
	}
}

func TestServiceDeleteIsIdempotent(t *testing.T) {
	svc, _ := newTestService()
	if err := svc.Delete(context.Background(), OwnerKindAlertRule, "alrule_never_saved"); err != nil {
		t.Fatalf("Delete() on an unsaved owner: error = %v, want nil", err)
	}
	if _, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", validDoc(), 0); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := svc.Delete(context.Background(), OwnerKindAlertRule, "alrule_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := svc.Delete(context.Background(), OwnerKindAlertRule, "alrule_1"); err != nil {
		t.Fatalf("second Delete() error = %v, want nil (idempotent)", err)
	}
	_, found, _ := svc.Get(context.Background(), OwnerKindAlertRule, "alrule_1")
	if found {
		t.Fatal("Get() found = true after Delete, want false")
	}
}

func TestServiceOwnersAreIsolated(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Save(context.Background(), OwnerKindAlertRule, "alrule_1", validDoc(), 0); err != nil {
		t.Fatalf("Save() alrule_1 error = %v", err)
	}
	_, found, err := svc.Get(context.Background(), OwnerKindAlertRule, "alrule_2")
	if err != nil {
		t.Fatalf("Get() alrule_2 error = %v", err)
	}
	if found {
		t.Fatal("Get() alrule_2 found = true, want false - saving alrule_1 must never affect a different owner")
	}
}
