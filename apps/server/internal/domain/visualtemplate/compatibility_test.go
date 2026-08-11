package visualtemplate

import (
	"errors"
	"testing"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

func TestAssessCompatibilityTargetMismatch(t *testing.T) {
	tpl := validTemplate(TargetChat)
	c := AssessCompatibility(tpl, TargetAlert, nil)
	if c.Compatible {
		t.Fatal("expected incompatible for a target mismatch")
	}
	if len(c.Blockers) != 1 || c.Blockers[0] != BlockerTargetMismatch {
		t.Errorf("blockers = %v, want [%s]", c.Blockers, BlockerTargetMismatch)
	}
}

func TestAssessCompatibilityNoOwnerCheckMeansTargetOnly(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	c := AssessCompatibility(tpl, TargetAlert, nil)
	if !c.Compatible {
		t.Fatalf("expected compatible, got blockers %v", c.Blockers)
	}
}

func TestAssessCompatibilityOwnerCheckRejectsAlert(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	check := func(doc visualdesign.Document) error { return errors.New("quantity unavailable for follow") }
	c := AssessCompatibility(tpl, TargetAlert, check)
	if c.Compatible {
		t.Fatal("expected incompatible")
	}
	if len(c.Blockers) != 1 || c.Blockers[0] != BlockerAlertBindingUnavailable {
		t.Errorf("blockers = %v, want [%s]", c.Blockers, BlockerAlertBindingUnavailable)
	}
}

func TestAssessCompatibilityOwnerCheckRejectsChat(t *testing.T) {
	tpl := validTemplate(TargetChat)
	check := func(doc visualdesign.Document) error { return errors.New("binding unavailable") }
	c := AssessCompatibility(tpl, TargetChat, check)
	if len(c.Blockers) != 1 || c.Blockers[0] != BlockerChatBindingUnavailable {
		t.Errorf("blockers = %v, want [%s]", c.Blockers, BlockerChatBindingUnavailable)
	}
}

func TestAssessCompatibilityOwnerCheckPasses(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	check := func(doc visualdesign.Document) error { return nil }
	c := AssessCompatibility(tpl, TargetAlert, check)
	if !c.Compatible {
		t.Fatalf("expected compatible, got blockers %v", c.Blockers)
	}
}

func TestAssessCompatibilityUnsupportedDocumentVersion(t *testing.T) {
	tpl := validTemplate(TargetAlert)
	tpl.Document.Version = 999
	c := AssessCompatibility(tpl, TargetAlert, nil)
	if c.Compatible || len(c.Blockers) != 1 || c.Blockers[0] != BlockerUnsupportedDocument {
		t.Errorf("blockers = %v, want [%s]", c.Blockers, BlockerUnsupportedDocument)
	}
}
