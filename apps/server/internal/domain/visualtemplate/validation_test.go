package visualtemplate

import (
	"errors"
	"strings"
	"testing"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

func validAlertDoc() visualdesign.Document {
	return visualdesign.Document{
		Version: visualdesign.CurrentVersion,
		Canvas:  visualdesign.CanvasLandscape,
		Layers: []visualdesign.Layer{
			builtinTextLayer("layer_1", "Text", visualdesign.Frame{X: 0, Y: 0, Width: 400, Height: 60}, 0, visualdesign.BindingUsername, 32, "#FFFFFF", 400),
		},
	}
}

func validTemplate(target Target) Template {
	doc := validAlertDoc()
	if target == TargetChat {
		doc.Canvas = visualdesign.CanvasChatItem
	}
	return Template{
		ID: "tpl_test", Target: target, Source: SourceUser,
		Name: "Test Template", Description: "desc", Author: "me", License: "MIT",
		TemplateSchemaVersion: CurrentTemplateSchemaVersion, Document: doc,
	}
}

func TestValidateAcceptsAValidTemplate(t *testing.T) {
	if err := Validate(validTemplate(TargetAlert)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := Validate(validTemplate(TargetChat)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsUnknownTarget(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	tpl.Target = "widget"
	if err := Validate(tpl); !errors.Is(err, ErrValidation) {
		t.Fatalf("got %v, want ErrValidation", err)
	}
}

func TestValidateRejectsEmptyName(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	tpl.Name = ""
	if err := Validate(tpl); err == nil {
		t.Fatal("expected an error for an empty name")
	}
}

func TestValidateRejectsOversizedName(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	tpl.Name = strings.Repeat("a", MaxNameLen+1)
	if err := Validate(tpl); err == nil {
		t.Fatal("expected an error for an oversized name")
	}
}

func TestValidateAcceptsUnicodeNameWithinBounds(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	tpl.Name = strings.Repeat("★", MaxNameLen)
	if err := Validate(tpl); err != nil {
		t.Fatalf("unexpected error for a bounded Unicode name: %v", err)
	}
}

func TestValidateRejectsOversizedDescription(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	tpl.Description = strings.Repeat("a", MaxDescriptionLen+1)
	if err := Validate(tpl); err == nil {
		t.Fatal("expected an error for an oversized description")
	}
}

func TestValidateRejectsUnsupportedTemplateSchemaVersion(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	tpl.TemplateSchemaVersion = 2
	if err := Validate(tpl); !errors.Is(err, ErrUnsupportedTemplateVersion) {
		t.Fatalf("got %v, want ErrUnsupportedTemplateVersion", err)
	}
}

func TestValidateRejectsAlertRenderedTextForChatTarget(t *testing.T) {
	tpl := validTemplate(TargetChat)
	tpl.Document.Layers[0].Text.Binding = visualdesign.BindingAlertRenderedText
	if err := Validate(tpl); err == nil {
		t.Fatal("expected an error for alert_rendered_text on a chat target")
	}
}

func TestValidateRejectsInvalidVisualDocument(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	tpl.Document.Layers[0].Frame.Width = 2 // below MinLayerSize
	if err := Validate(tpl); !errors.Is(err, ErrValidation) {
		t.Fatalf("got %v, want ErrValidation", err)
	}
}

func TestNormalizeAndValidateDocumentMigratesVersion1(t *testing.T) {
	doc := validAlertDoc()
	doc.Version = visualdesign.Version1
	migrated, err := NormalizeAndValidateDocument(doc)
	if err != nil {
		t.Fatalf("unexpected error migrating a v1 document: %v", err)
	}
	if migrated.Version != visualdesign.CurrentVersion {
		t.Errorf("Version = %d, want %d", migrated.Version, visualdesign.CurrentVersion)
	}
}

func TestNormalizeAndValidateDocumentRejectsVersionZero(t *testing.T) {
	doc := validAlertDoc()
	doc.Version = 0
	if _, err := NormalizeAndValidateDocument(doc); !errors.Is(err, ErrUnsupportedDesignVersion) {
		t.Fatalf("got %v, want ErrUnsupportedDesignVersion", err)
	}
}

func TestNormalizeAndValidateDocumentRejectsFutureVersion(t *testing.T) {
	doc := validAlertDoc()
	doc.Version = visualdesign.CurrentVersion + 1
	if _, err := NormalizeAndValidateDocument(doc); !errors.Is(err, ErrUnsupportedDesignVersion) {
		t.Fatalf("got %v, want ErrUnsupportedDesignVersion", err)
	}
}
