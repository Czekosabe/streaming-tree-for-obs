package visualdesign

import "testing"

func validDoc() Document {
	return Document{
		Version: CurrentVersion,
		Canvas:  CanvasLandscape,
		Layers: []Layer{
			{
				ID: "layer_1", Name: "Background", Kind: LayerShape,
				Visible: true, Locked: false, Order: 0,
				Frame: Frame{X: 0, Y: 0, Width: 400, Height: 200}, Opacity: 1,
				Shape:          &ShapeProps{Kind: ShapeRectangle, Fill: "#112233", BorderColor: "#000000", BorderWidth: 0, CornerRadius: 8},
				EntryAnimation: AnimationFade, ExitAnimation: AnimationFade, AnimationDurationMS: 300,
			},
			{
				ID: "layer_2", Name: "Text", Kind: LayerText,
				Visible: true, Locked: false, Order: 1,
				Frame: Frame{X: 10, Y: 10, Width: 380, Height: 100}, Opacity: 1,
				Text: &TextProps{
					Binding: BindingAlertRenderedText, MissingValueBehavior: MissingHide,
					FontFamily: FontSystemUI, FontSize: 32, FontWeight: 700, LineHeight: 1.2, LetterSpacing: 0,
					TextColor: "#FFFFFF", HorizontalAlign: HAlignCenter, VerticalAlign: VAlignMiddle,
					OutlineWidth: 0, OutlineColor: "#000000",
				},
				EntryAnimation: AnimationNone, ExitAnimation: AnimationNone, AnimationDurationMS: 0,
			},
		},
	}
}

func TestValidateAcceptsAWellFormedDocument(t *testing.T) {
	if err := Validate(validDoc()); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidateRejectsUnknownVersion(t *testing.T) {
	doc := validDoc()
	doc.Version = 999
	if err := Validate(doc); err == nil {
		t.Fatal("Validate() = nil, want an error for an unsupported version")
	}
}

func TestValidateCanvasBounds(t *testing.T) {
	tests := []struct {
		name string
		c    Canvas
		ok   bool
	}{
		{"min width/height", Canvas{Width: MinCanvasWidth, Height: MinCanvasHeight, Transparent: true}, true},
		{"max width/height", Canvas{Width: MaxCanvasWidth, Height: MaxCanvasHeight, Transparent: true}, true},
		{"width too small", Canvas{Width: MinCanvasWidth - 1, Height: 1080, Transparent: true}, false},
		{"height too small", Canvas{Width: 1920, Height: MinCanvasHeight - 1, Transparent: true}, false},
		{"width too large", Canvas{Width: MaxCanvasWidth + 1, Height: 1080, Transparent: true}, false},
		{"height too large", Canvas{Width: 1920, Height: MaxCanvasHeight + 1, Transparent: true}, false},
		{"landscape preset", CanvasLandscape, true},
		{"vertical preset", CanvasVertical, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Document{Version: CurrentVersion, Canvas: tt.c}
			err := Validate(doc)
			if (err == nil) != tt.ok {
				t.Errorf("Validate() error = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestValidateLayerCountBounds(t *testing.T) {
	base := validDoc()

	empty := base
	empty.Layers = nil
	if err := Validate(empty); err != nil {
		t.Errorf("0 layers: Validate() error = %v, want nil", err)
	}

	atMax := base
	atMax.Layers = make([]Layer, MaxLayers)
	for i := range atMax.Layers {
		l := base.Layers[1] // reuse the valid text layer shape
		l.ID = "layer_max_" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		l.Order = i
		atMax.Layers[i] = l
	}
	if err := Validate(atMax); err != nil {
		t.Errorf("%d layers: Validate() error = %v, want nil", MaxLayers, err)
	}

	overMax := base
	overMax.Layers = make([]Layer, MaxLayers+1)
	for i := range overMax.Layers {
		l := base.Layers[1]
		l.ID = "layer_over_" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		l.Order = i
		overMax.Layers[i] = l
	}
	if err := Validate(overMax); err == nil {
		t.Errorf("%d layers: Validate() = nil, want an error", MaxLayers+1)
	}
}

func TestValidateRejectsDuplicateLayerID(t *testing.T) {
	doc := validDoc()
	doc.Layers[1].ID = doc.Layers[0].ID
	if err := Validate(doc); err == nil {
		t.Fatal("Validate() = nil, want an error for a duplicate layer id")
	}
}

func TestValidateRejectsOverlongLayerName(t *testing.T) {
	doc := validDoc()
	name := ""
	for i := 0; i < MaxLayerNameCodePoints+1; i++ {
		name += "a"
	}
	doc.Layers[0].Name = name
	if err := Validate(doc); err == nil {
		t.Fatal("Validate() = nil, want an error for an overlong layer name")
	}
}

func TestValidateShapeLayer(t *testing.T) {
	doc := validDoc()
	if err := Validate(doc); err != nil {
		t.Fatalf("valid shape layer: Validate() error = %v", err)
	}

	bad := validDoc()
	bad.Layers[0].Shape.Fill = "blue"
	if err := Validate(bad); err == nil {
		t.Error("invalid fill color: Validate() = nil, want an error")
	}

	badRadius := validDoc()
	badRadius.Layers[0].Shape.CornerRadius = MaxCornerRadius + 1
	if err := Validate(badRadius); err == nil {
		t.Error("corner radius over max: Validate() = nil, want an error")
	}
}

func TestValidateTextStaticLayer(t *testing.T) {
	doc := validDoc()
	doc.Layers[1].Text.Binding = BindingStatic
	doc.Layers[1].Text.StaticText = "Hello!"
	if err := Validate(doc); err != nil {
		t.Fatalf("static text with content: Validate() error = %v", err)
	}

	empty := validDoc()
	empty.Layers[1].Text.Binding = BindingStatic
	empty.Layers[1].Text.StaticText = ""
	if err := Validate(empty); err == nil {
		t.Error("empty static text: Validate() = nil, want an error")
	}

	overlong := validDoc()
	text := ""
	for i := 0; i < MaxStaticTextCodePoints+1; i++ {
		text += "a"
	}
	overlong.Layers[1].Text.Binding = BindingStatic
	overlong.Layers[1].Text.StaticText = text
	if err := Validate(overlong); err == nil {
		t.Error("overlong static text: Validate() = nil, want an error")
	}
}

func TestValidateEveryAlertTextBinding(t *testing.T) {
	for _, binding := range []TextBinding{
		BindingAlertRenderedText, BindingUsername, BindingPlatform, BindingEventType,
		BindingMessage, BindingQuantity, BindingGroupCount,
	} {
		t.Run(string(binding), func(t *testing.T) {
			doc := validDoc()
			doc.Layers[1].Text.Binding = binding
			if err := Validate(doc); err != nil {
				t.Errorf("binding %q: Validate() error = %v, want nil", binding, err)
			}
		})
	}
}

func TestValidatePlatformIconLayer(t *testing.T) {
	doc := Document{
		Version: CurrentVersion, Canvas: CanvasLandscape,
		Layers: []Layer{{
			ID: "layer_icon", Name: "Icon", Kind: LayerPlatformIcon, Visible: true, Order: 0,
			Frame: Frame{X: 0, Y: 0, Width: 64, Height: 64}, Opacity: 1,
			PlatformIcon:   &PlatformIconProps{},
			EntryAnimation: AnimationNone, ExitAnimation: AnimationNone,
		}},
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
}

func TestValidateAvatarLayer(t *testing.T) {
	doc := Document{
		Version: CurrentVersion, Canvas: CanvasLandscape,
		Layers: []Layer{{
			ID: "layer_avatar", Name: "Avatar", Kind: LayerAvatar, Visible: true, Order: 0,
			Frame: Frame{X: 0, Y: 0, Width: 96, Height: 96}, Opacity: 1,
			Avatar:         &AvatarProps{CornerRadius: 48, BorderColor: "#FFFFFF", BorderWidth: 2},
			EntryAnimation: AnimationNone, ExitAnimation: AnimationNone,
		}},
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}

	bad := doc
	badAvatar := *doc.Layers[0].Avatar
	badAvatar.BorderWidth = 2
	badAvatar.BorderColor = "not-a-color"
	bad.Layers = []Layer{doc.Layers[0]}
	bad.Layers[0].Avatar = &badAvatar
	if err := Validate(bad); err == nil {
		t.Error("invalid avatar border color: Validate() = nil, want an error")
	}
}

func TestValidateMessageFragmentsLayer(t *testing.T) {
	doc := Document{
		Version: CurrentVersion, Canvas: CanvasLandscape,
		Layers: []Layer{{
			ID: "layer_fragments", Name: "Message", Kind: LayerMessageFragments, Visible: true, Order: 0,
			Frame: Frame{X: 0, Y: 0, Width: 400, Height: 100}, Opacity: 1,
			MessageFragments: &MessageFragmentsProps{
				FontFamily: FontSystemUI, FontSize: 16, FontWeight: 400, LineHeight: 1.2, LetterSpacing: 0,
				TextColor: "#FFFFFF", HorizontalAlign: HAlignLeft, VerticalAlign: VAlignTop, EmoteSize: 24,
			},
			EntryAnimation: AnimationNone, ExitAnimation: AnimationNone,
		}},
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}

	badEmote := doc
	badEmote.Layers = []Layer{doc.Layers[0]}
	fragCopy := *doc.Layers[0].MessageFragments
	fragCopy.EmoteSize = MaxEmoteSize + 1
	badEmote.Layers[0].MessageFragments = &fragCopy
	if err := Validate(badEmote); err == nil {
		t.Error("emote size over max: Validate() = nil, want an error")
	}

	badColor := doc
	badColor.Layers = []Layer{doc.Layers[0]}
	fragCopy2 := *doc.Layers[0].MessageFragments
	fragCopy2.TextColor = "not-a-color"
	badColor.Layers[0].MessageFragments = &fragCopy2
	if err := Validate(badColor); err == nil {
		t.Error("invalid text color: Validate() = nil, want an error")
	}
}

func TestValidateBadgeListLayer(t *testing.T) {
	doc := Document{
		Version: CurrentVersion, Canvas: CanvasLandscape,
		Layers: []Layer{{
			ID: "layer_badges", Name: "Badges", Kind: LayerBadgeList, Visible: true, Order: 0,
			Frame: Frame{X: 0, Y: 0, Width: 200, Height: 32}, Opacity: 1,
			BadgeList:      &BadgeListProps{MaxCount: 5, BadgeSize: 24, Gap: 4},
			EntryAnimation: AnimationNone, ExitAnimation: AnimationNone,
		}},
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}

	tests := []struct {
		name  string
		apply func(*BadgeListProps)
	}{
		{"max count too low", func(b *BadgeListProps) { b.MaxCount = MinBadgeCount - 1 }},
		{"max count too high", func(b *BadgeListProps) { b.MaxCount = MaxBadgeCount + 1 }},
		{"badge size too small", func(b *BadgeListProps) { b.BadgeSize = MinBadgeSize - 1 }},
		{"badge size too large", func(b *BadgeListProps) { b.BadgeSize = MaxBadgeSize + 1 }},
		{"gap negative", func(b *BadgeListProps) { b.Gap = MinBadgeGap - 1 }},
		{"gap too large", func(b *BadgeListProps) { b.Gap = MaxBadgeGap + 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bad := doc
			bad.Layers = []Layer{doc.Layers[0]}
			badgeCopy := *doc.Layers[0].BadgeList
			tt.apply(&badgeCopy)
			bad.Layers[0].BadgeList = &badgeCopy
			if err := Validate(bad); err == nil {
				t.Error("Validate() = nil, want an error")
			}
		})
	}
}

func TestValidateNewStage13BTextBindings(t *testing.T) {
	for _, binding := range []TextBinding{BindingTimestamp, BindingAccountLabel} {
		t.Run(string(binding), func(t *testing.T) {
			doc := validDoc()
			doc.Layers[1].Text.Binding = binding
			if err := Validate(doc); err != nil {
				t.Errorf("binding %q: Validate() error = %v, want nil", binding, err)
			}
		})
	}
}

func TestValidateRejectsUnrecognizedLayerKind(t *testing.T) {
	doc := validDoc()
	doc.Layers[0].Kind = LayerKind("video")
	doc.Layers[0].Shape = nil
	if err := Validate(doc); err == nil {
		t.Fatal("Validate() = nil, want an error for an unrecognized layer kind")
	}
}

func TestValidateRejectsMismatchedPayload(t *testing.T) {
	doc := validDoc()
	// Kind says shape, but the payload present is Text.
	doc.Layers[0].Shape = nil
	doc.Layers[0].Text = &TextProps{
		Binding: BindingStatic, StaticText: "x", MissingValueBehavior: MissingHide,
		FontFamily: FontSystemUI, FontSize: 16, FontWeight: 400, LineHeight: 1, TextColor: "#000000",
		HorizontalAlign: HAlignLeft, VerticalAlign: VAlignTop,
	}
	if err := Validate(doc); err == nil {
		t.Fatal("Validate() = nil, want an error when the payload does not match Kind")
	}
}

func TestValidateFrameBounds(t *testing.T) {
	tests := []struct {
		name string
		f    Frame
		ok   bool
	}{
		{"min size", Frame{X: 0, Y: 0, Width: MinLayerSize, Height: MinLayerSize}, true},
		{"too small width", Frame{X: 0, Y: 0, Width: MinLayerSize - 1, Height: 100}, false},
		{"too small height", Frame{X: 0, Y: 0, Width: 100, Height: MinLayerSize - 1}, false},
		{"zero size", Frame{X: 0, Y: 0, Width: 0, Height: 0}, false},
		{"negative position", Frame{X: -1, Y: 0, Width: 100, Height: 100}, false},
		{"off canvas right", Frame{X: 1900, Y: 0, Width: 100, Height: 100}, false},
		{"off canvas bottom", Frame{X: 0, Y: 1060, Width: 100, Height: 100}, false},
		{"fits exactly", Frame{X: 1820, Y: 980, Width: 100, Height: 100}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := validDoc()
			doc.Layers[0].Frame = tt.f
			err := Validate(doc)
			if (err == nil) != tt.ok {
				t.Errorf("Validate() error = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestValidateOpacityBounds(t *testing.T) {
	for _, v := range []float64{-0.1, 1.1} {
		doc := validDoc()
		doc.Layers[0].Opacity = v
		if err := Validate(doc); err == nil {
			t.Errorf("opacity %v: Validate() = nil, want an error", v)
		}
	}
	for _, v := range []float64{0, 1, 0.5} {
		doc := validDoc()
		doc.Layers[0].Opacity = v
		if err := Validate(doc); err != nil {
			t.Errorf("opacity %v: Validate() error = %v, want nil", v, err)
		}
	}
}

func TestValidateFontBounds(t *testing.T) {
	tooSmall := validDoc()
	tooSmall.Layers[1].Text.FontSize = MinFontSize - 1
	if err := Validate(tooSmall); err == nil {
		t.Error("font size too small: Validate() = nil, want an error")
	}

	tooLarge := validDoc()
	tooLarge.Layers[1].Text.FontSize = MaxFontSize + 1
	if err := Validate(tooLarge); err == nil {
		t.Error("font size too large: Validate() = nil, want an error")
	}

	badWeight := validDoc()
	badWeight.Layers[1].Text.FontWeight = 450
	if err := Validate(badWeight); err == nil {
		t.Error("non-increment font weight: Validate() = nil, want an error")
	}

	unknownFont := validDoc()
	unknownFont.Layers[1].Text.FontFamily = FontFamily("Comic Sans")
	if err := Validate(unknownFont); err == nil {
		t.Error("arbitrary font family: Validate() = nil, want an error")
	}
}

func TestValidateShadowBoundsOnlyWhenEnabled(t *testing.T) {
	disabled := validDoc()
	disabled.Layers[1].Text.ShadowEnabled = false
	disabled.Layers[1].Text.ShadowBlur = -999 // would be invalid if checked
	if err := Validate(disabled); err != nil {
		t.Errorf("shadow disabled: Validate() error = %v, want nil (unchecked fields ignored)", err)
	}

	enabled := validDoc()
	enabled.Layers[1].Text.ShadowEnabled = true
	enabled.Layers[1].Text.ShadowBlur = MaxShadowBlur + 1
	enabled.Layers[1].Text.ShadowColor = "#000000"
	if err := Validate(enabled); err == nil {
		t.Error("shadow enabled with out-of-bounds blur: Validate() = nil, want an error")
	}
}

func TestValidateAnimationDurationBounds(t *testing.T) {
	tooLong := validDoc()
	tooLong.Layers[0].AnimationDurationMS = MaxLayerAnimationDurationMS + 1
	if err := Validate(tooLong); err == nil {
		t.Error("animation duration too long: Validate() = nil, want an error")
	}
}

func TestIsValidColor(t *testing.T) {
	valid := []string{"#000000", "#FFFFFF", "#a1b2c3", "#a1b2c3d4"}
	for _, c := range valid {
		if !IsValidColor(c) {
			t.Errorf("IsValidColor(%q) = false, want true", c)
		}
	}
	invalid := []string{"red", "rgb(0,0,0)", "#fff", "#gggggg", "javascript:alert(1)"}
	for _, c := range invalid {
		if IsValidColor(c) {
			t.Errorf("IsValidColor(%q) = true, want false", c)
		}
	}
}
