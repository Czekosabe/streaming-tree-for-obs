package visualtemplate

import (
	"context"
	"errors"
	"testing"
)

type fakeRepository struct {
	items map[string]Template
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{items: map[string]Template{}}
}

func (f *fakeRepository) Create(_ context.Context, t Template) (Template, error) {
	f.items[t.ID] = t
	return t, nil
}

func (f *fakeRepository) Get(_ context.Context, id string) (Template, error) {
	t, ok := f.items[id]
	if !ok {
		return Template{}, ErrNotFound
	}
	return t, nil
}

func (f *fakeRepository) List(_ context.Context) ([]Template, error) {
	out := make([]Template, 0, len(f.items))
	for _, t := range f.items {
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeRepository) UpdateMetadata(_ context.Context, id, name, description, author, license string) (Template, error) {
	t, ok := f.items[id]
	if !ok {
		return Template{}, ErrNotFound
	}
	t.Name, t.Description, t.Author, t.License = name, description, author, license
	f.items[id] = t
	return t, nil
}

func (f *fakeRepository) Delete(_ context.Context, id string) error {
	delete(f.items, id)
	return nil
}

func TestNewServiceRejectsInvalidBuiltins(t *testing.T) {
	bad := []Template{{ID: "tpl_bad", Target: TargetAlert, Source: SourceBuiltin}}
	if _, err := NewService(newFakeRepository(), bad, nil); err == nil {
		t.Fatal("expected NewService to reject an invalid built-in set")
	}
}

func TestServiceListMergesBuiltinsAndUsers(t *testing.T) {
	svc, _ := testServiceSimple(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, TargetAlert, "Mine", "", "", "", validAlertDoc()); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != len(DefaultBuiltins())+1 {
		t.Errorf("List() len = %d, want %d", len(list), len(DefaultBuiltins())+1)
	}
}

func TestServiceCreateGeneratesTplID(t *testing.T) {
	svc, _ := testServiceSimple(t)
	created, err := svc.Create(context.Background(), TargetAlert, "Mine", "", "", "", validAlertDoc())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(created.ID) < 4 || created.ID[:4] != "tpl_" {
		t.Errorf("created.ID = %q, want a tpl_-prefixed id", created.ID)
	}
}

func TestServiceCreateMigratesV1Document(t *testing.T) {
	svc, _ := testServiceSimple(t)
	doc := validAlertDoc()
	doc.Version = 1
	created, err := svc.Create(context.Background(), TargetAlert, "Mine", "", "", "", doc)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Document.Version != 2 {
		t.Errorf("Document.Version = %d, want 2", created.Document.Version)
	}
}

func TestServiceGetReturnsBuiltinWithoutRepository(t *testing.T) {
	svc, _ := testServiceSimple(t)
	got, err := svc.Get(context.Background(), "builtin_alert_minimal_dark")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Source != SourceBuiltin {
		t.Errorf("Source = %v, want SourceBuiltin", got.Source)
	}
}

func TestServiceUpdateMetadataRejectsBuiltin(t *testing.T) {
	svc, _ := testServiceSimple(t)
	_, err := svc.UpdateMetadata(context.Background(), "builtin_alert_minimal_dark", "x", "y", "z", "w")
	if !errors.Is(err, ErrImmutable) {
		t.Fatalf("got %v, want ErrImmutable", err)
	}
}

func TestServiceDeleteRejectsBuiltin(t *testing.T) {
	svc, _ := testServiceSimple(t)
	if err := svc.Delete(context.Background(), "builtin_alert_minimal_dark"); !errors.Is(err, ErrImmutable) {
		t.Fatalf("got %v, want ErrImmutable", err)
	}
}

func TestServiceDeleteUserTemplate(t *testing.T) {
	svc, _ := testServiceSimple(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, TargetAlert, "Mine", "", "", "", validAlertDoc())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestServiceImportPreviewDoesNotPersist(t *testing.T) {
	svc, repo := testServiceSimple(t)
	candidate := Template{
		Target: TargetAlert, Name: "Imported", TemplateSchemaVersion: CurrentTemplateSchemaVersion, Document: validAlertDoc(),
	}
	if _, err := svc.ImportPreview(candidate); err != nil {
		t.Fatalf("ImportPreview() error = %v", err)
	}
	if len(repo.items) != 0 {
		t.Errorf("ImportPreview must not persist, repo has %d items", len(repo.items))
	}
}

func TestServiceImportPersistsWithNewID(t *testing.T) {
	svc, _ := testServiceSimple(t)
	candidate := Template{
		ID: "some-untrusted-client-supplied-id", Target: TargetAlert, Name: "Imported",
		TemplateSchemaVersion: CurrentTemplateSchemaVersion, Document: validAlertDoc(),
	}
	imported, err := svc.Import(context.Background(), candidate)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if imported.ID == "some-untrusted-client-supplied-id" {
		t.Error("Import must never trust a client-supplied id")
	}
	if len(imported.ID) < 4 || imported.ID[:4] != "tpl_" {
		t.Errorf("imported.ID = %q, want a tpl_-prefixed id", imported.ID)
	}
}

func TestServiceImportRejectsUnsupportedDesignVersion(t *testing.T) {
	svc, _ := testServiceSimple(t)
	doc := validAlertDoc()
	doc.Version = 999
	candidate := Template{Target: TargetAlert, Name: "Bad", TemplateSchemaVersion: CurrentTemplateSchemaVersion, Document: doc}
	if _, err := svc.Import(context.Background(), candidate); !errors.Is(err, ErrUnsupportedDesignVersion) {
		t.Fatalf("got %v, want ErrUnsupportedDesignVersion", err)
	}
}

func TestServiceExportWorksForBuiltinAndUser(t *testing.T) {
	svc, _ := testServiceSimple(t)
	ctx := context.Background()
	if _, err := svc.Export(ctx, "builtin_chat_compact"); err != nil {
		t.Fatalf("Export(builtin) error = %v", err)
	}
	created, err := svc.Create(ctx, TargetAlert, "Mine", "", "", "", validAlertDoc())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.Export(ctx, created.ID); err != nil {
		t.Fatalf("Export(user) error = %v", err)
	}
}

func testServiceSimple(t *testing.T) (*Service, *fakeRepository) {
	t.Helper()
	repo := newFakeRepository()
	svc, err := NewService(repo, DefaultBuiltins(), nil)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc, repo
}
