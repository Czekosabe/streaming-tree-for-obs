package chatoverlay

import (
	"errors"
	"testing"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

func TestAvailableTextBindingsMessageItem(t *testing.T) {
	bindings := AvailableTextBindings(ItemKindMessage)
	want := map[visualdesign.TextBinding]bool{visualdesign.BindingMessage: true}
	notWant := map[visualdesign.TextBinding]bool{
		visualdesign.BindingEventType: true, visualdesign.BindingQuantity: true,
		visualdesign.BindingAlertRenderedText: true, visualdesign.BindingGroupCount: true,
	}
	assertBindingSets(t, bindings, want, notWant)
}

func TestAvailableTextBindingsActivityItem(t *testing.T) {
	bindings := AvailableTextBindings(ItemKindActivity)
	want := map[visualdesign.TextBinding]bool{visualdesign.BindingEventType: true, visualdesign.BindingQuantity: true}
	notWant := map[visualdesign.TextBinding]bool{
		visualdesign.BindingMessage: true, visualdesign.BindingAlertRenderedText: true, visualdesign.BindingGroupCount: true,
	}
	assertBindingSets(t, bindings, want, notWant)
}

func assertBindingSets(t *testing.T, got []visualdesign.TextBinding, want, notWant map[visualdesign.TextBinding]bool) {
	t.Helper()
	set := make(map[visualdesign.TextBinding]bool, len(got))
	for _, b := range got {
		set[b] = true
	}
	for b := range want {
		if !set[b] {
			t.Errorf("binding %q missing, want present", b)
		}
	}
	for b := range notWant {
		if set[b] {
			t.Errorf("binding %q present, want absent", b)
		}
	}
}

func chatDesignWithBinding(binding visualdesign.TextBinding) visualdesign.Document {
	return visualdesign.Document{
		Version: visualdesign.CurrentVersion, Canvas: visualdesign.CanvasChatItem,
		Layers: []visualdesign.Layer{{
			ID: "layer_1", Kind: visualdesign.LayerText, Visible: true, Order: 0,
			Frame: visualdesign.Frame{X: 0, Y: 0, Width: 400, Height: 100}, Opacity: 1,
			Text: &visualdesign.TextProps{
				Binding: binding, MissingValueBehavior: visualdesign.MissingHide,
				FontFamily: visualdesign.FontSystemUI, FontSize: 16, FontWeight: 400, LineHeight: 1.2,
				TextColor: "#FFFFFF", HorizontalAlign: visualdesign.HAlignLeft, VerticalAlign: visualdesign.VAlignTop,
			},
			EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
		}},
	}
}

func TestValidateDesignBindingsForChatOverlayAcceptsChatBindings(t *testing.T) {
	for _, b := range []visualdesign.TextBinding{
		visualdesign.BindingUsername, visualdesign.BindingPlatform, visualdesign.BindingMessage,
		visualdesign.BindingTimestamp, visualdesign.BindingAccountLabel, visualdesign.BindingEventType, visualdesign.BindingQuantity,
	} {
		t.Run(string(b), func(t *testing.T) {
			if err := ValidateDesignBindingsForChatOverlay(chatDesignWithBinding(b)); err != nil {
				t.Errorf("binding %q rejected: %v", b, err)
			}
		})
	}
}

func TestValidateDesignBindingsForChatOverlayRejectsAlertOnlyBindings(t *testing.T) {
	for _, b := range []visualdesign.TextBinding{visualdesign.BindingAlertRenderedText, visualdesign.BindingGroupCount} {
		t.Run(string(b), func(t *testing.T) {
			err := ValidateDesignBindingsForChatOverlay(chatDesignWithBinding(b))
			if !errors.Is(err, visualdesign.ErrValidation) {
				t.Errorf("binding %q: err = %v, want ErrValidation", b, err)
			}
		})
	}
}
