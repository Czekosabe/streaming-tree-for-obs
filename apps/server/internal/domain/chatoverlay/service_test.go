package chatoverlay

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepository is a minimal in-memory Repository for service-level
// tests that do not need real SQLite persistence semantics (uniqueness,
// cascade) already covered by internal/storage/sqlite's own tests.
type fakeRepository struct {
	profiles     map[string]Profile
	blockedTerms map[string][]BlockedTerm
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{profiles: map[string]Profile{}, blockedTerms: map[string][]BlockedTerm{}}
}

func (f *fakeRepository) CreateProfile(_ context.Context, p Profile) (Profile, error) {
	if _, exists := f.profiles[p.ID]; exists {
		return Profile{}, errors.New("duplicate id")
	}
	f.profiles[p.ID] = p
	return p, nil
}
func (f *fakeRepository) GetProfile(_ context.Context, id string) (Profile, bool, error) {
	p, ok := f.profiles[id]
	return p, ok, nil
}
func (f *fakeRepository) GetProfileByPublicSlug(_ context.Context, slug string) (Profile, bool, error) {
	for _, p := range f.profiles {
		if p.PublicSlug == slug {
			return p, true, nil
		}
	}
	return Profile{}, false, nil
}
func (f *fakeRepository) ListProfiles(_ context.Context) ([]Profile, error) {
	out := make([]Profile, 0, len(f.profiles))
	for _, p := range f.profiles {
		out = append(out, p)
	}
	return out, nil
}
func (f *fakeRepository) UpdateProfile(_ context.Context, p Profile) (Profile, error) {
	if _, ok := f.profiles[p.ID]; !ok {
		return Profile{}, ErrNotFound
	}
	f.profiles[p.ID] = p
	return p, nil
}
func (f *fakeRepository) DeleteProfile(_ context.Context, id string) error {
	delete(f.profiles, id)
	return nil
}
func (f *fakeRepository) RotatePublicSlug(_ context.Context, id, newSlug string, _ time.Time) (Profile, error) {
	p, ok := f.profiles[id]
	if !ok {
		return Profile{}, ErrNotFound
	}
	p.PublicSlug = newSlug
	f.profiles[id] = p
	return p, nil
}
func (f *fakeRepository) ListAccounts(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (f *fakeRepository) SetAccounts(_ context.Context, _ string, _ []string) error  { return nil }
func (f *fakeRepository) ListHiddenUsers(_ context.Context, _ string) ([]HiddenUser, error) {
	return nil, nil
}
func (f *fakeRepository) AddHiddenUser(_ context.Context, ref HiddenUser, _ time.Time) (HiddenUser, error) {
	return ref, nil
}
func (f *fakeRepository) RemoveHiddenUser(_ context.Context, _ string, _ ProviderID, _, _ string) error {
	return nil
}
func (f *fakeRepository) ListBlockedTerms(_ context.Context, overlayID string) ([]BlockedTerm, error) {
	return f.blockedTerms[overlayID], nil
}
func (f *fakeRepository) AddBlockedTerm(_ context.Context, term BlockedTerm, _ time.Time) (BlockedTerm, error) {
	f.blockedTerms[term.OverlayID] = append(f.blockedTerms[term.OverlayID], term)
	return term, nil
}
func (f *fakeRepository) RemoveBlockedTerm(_ context.Context, _, _ string) error { return nil }
func (f *fakeRepository) ListActivityTypes(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakeRepository) SetActivityTypes(_ context.Context, _ string, _ []string) error { return nil }

func TestServiceCreateProfileGeneratesValidIDAndSlug(t *testing.T) {
	svc := NewService(newFakeRepository(), nil)
	p, err := svc.CreateProfile(context.Background(), "My Overlay")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if p.ID == "" || p.PublicSlug == "" {
		t.Fatalf("CreateProfile() = %+v, want a non-empty id and public slug", p)
	}
	if p.ID == p.PublicSlug {
		t.Error("id and public slug must never be the same value")
	}
}

func TestServiceCreateProfileRejectsEmptyName(t *testing.T) {
	svc := NewService(newFakeRepository(), nil)
	if _, err := svc.CreateProfile(context.Background(), "   "); err == nil {
		t.Fatal("expected a validation error for an empty name")
	}
}

func TestServiceGetProfileNotFound(t *testing.T) {
	svc := NewService(newFakeRepository(), nil)
	if _, err := svc.GetProfile(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetProfile() error = %v, want ErrNotFound", err)
	}
}

func TestServiceRotatePublicSlugChangesTheURL(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)
	created, err := svc.CreateProfile(context.Background(), "x")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	rotated, err := svc.RotatePublicSlug(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("RotatePublicSlug() error = %v", err)
	}
	if rotated.PublicSlug == created.PublicSlug {
		t.Error("expected a different public slug after rotation")
	}
}

func TestServiceAddBlockedTermEnforcesPerOverlayLimit(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)

	for i := 0; i < MaxBlockedTermsPerOverlay; i++ {
		value := string(rune('a'+i%26)) + string(rune('0'+i/26))
		if _, err := svc.AddBlockedTerm(context.Background(), "ov_1", value, MatchContains); err != nil {
			t.Fatalf("AddBlockedTerm() #%d error = %v", i, err)
		}
	}

	if _, err := svc.AddBlockedTerm(context.Background(), "ov_1", "one_too_many", MatchContains); err == nil {
		t.Fatal("expected an error once the per-overlay blocked-term limit is exceeded")
	}
}

func TestServiceAddBlockedTermRejectsInvalidValueBeforeTouchingTheRepository(t *testing.T) {
	svc := NewService(newFakeRepository(), nil)
	if _, err := svc.AddBlockedTerm(context.Background(), "ov_1", "", MatchContains); err == nil {
		t.Fatal("expected a validation error for an empty term")
	}
}
