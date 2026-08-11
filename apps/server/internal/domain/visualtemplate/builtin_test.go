package visualtemplate

import "testing"

func TestDefaultBuiltinsPassValidation(t *testing.T) {
	if err := ValidateBuiltins(DefaultBuiltins()); err != nil {
		t.Fatalf("DefaultBuiltins() failed validation: %v", err)
	}
}

func TestDefaultBuiltinsHasAtLeastThreeAlertAndThreeChat(t *testing.T) {
	var alertCount, chatCount int
	for _, b := range DefaultBuiltins() {
		switch b.Target {
		case TargetAlert:
			alertCount++
		case TargetChat:
			chatCount++
		}
	}
	if alertCount < 3 {
		t.Errorf("alert built-ins = %d, want >= 3", alertCount)
	}
	if chatCount < 3 {
		t.Errorf("chat built-ins = %d, want >= 3", chatCount)
	}
}

func TestValidateBuiltinsRejectsDuplicateID(t *testing.T) {
	builtins := DefaultBuiltins()
	dup := builtins[0]
	builtins = append(builtins, dup)
	if err := ValidateBuiltins(builtins); err == nil {
		t.Fatal("expected an error for a duplicate built-in id")
	}
}

func TestValidateBuiltinsRejectsTplNamespaceCollision(t *testing.T) {
	bad := builtinAlertTemplate("tpl_should_not_be_builtin", "Bad", "", "#000000", "#FFFFFF", "#FFFFFF")
	if err := ValidateBuiltins([]Template{bad}); err == nil {
		t.Fatal("expected an error for a built-in id using the tpl_ namespace")
	}
}

func TestValidateBuiltinsRejectsInvalidDocument(t *testing.T) {
	bad := DefaultBuiltins()[0]
	bad.Document.Layers = append(bad.Document.Layers, bad.Document.Layers[0]) // duplicate layer id
	if err := ValidateBuiltins([]Template{bad}); err == nil {
		t.Fatal("expected an error for an invalid built-in document")
	}
}

func TestNoBuiltinUsesAlertRenderedTextForChatTarget(t *testing.T) {
	for _, b := range DefaultBuiltins() {
		if b.Target != TargetChat {
			continue
		}
		if err := Validate(b); err != nil {
			t.Errorf("chat built-in %q failed validation: %v", b.ID, err)
		}
	}
}
