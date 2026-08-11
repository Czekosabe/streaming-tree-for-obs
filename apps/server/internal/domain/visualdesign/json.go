package visualdesign

import "encoding/json"

// MarshalDocumentJSON/UnmarshalDocumentJSON are the one shared,
// exported wire mirror for a Document's own JSON shape (Stage 14A) -
// used by any storage layer that needs to persist a Document as a JSON
// column without re-deriving its own private mirror struct (Stage 14A
// task Part 26/27: internal/storage/sqlite's own visual_templates
// repository, alongside internal/storage/sqlite's pre-existing
// visual_designs repository, which keeps its own long-standing private
// mirror unchanged - this file adds a reusable option, it does not
// replace that one). The wire shape is identical either way: the same
// lowerCamelCase field names produced by internal/httpapi's own
// management DTOs.
type jsonDocument struct {
	Version int         `json:"version"`
	Canvas  jsonCanvas  `json:"canvas"`
	Layers  []jsonLayer `json:"layers"`
}

type jsonCanvas struct {
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	Transparent bool `json:"transparent"`
}

type jsonFrame struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type jsonShapeProps struct {
	Kind         string `json:"kind"`
	Fill         string `json:"fill"`
	BorderColor  string `json:"borderColor"`
	BorderWidth  int    `json:"borderWidth"`
	CornerRadius int    `json:"cornerRadius"`
}

type jsonTextProps struct {
	Binding              string  `json:"binding"`
	StaticText           string  `json:"staticText"`
	MissingValueBehavior string  `json:"missingValueBehavior"`
	FontFamily           string  `json:"fontFamily"`
	FontAssetID          string  `json:"fontAssetId,omitempty"`
	FontSize             int     `json:"fontSize"`
	FontWeight           int     `json:"fontWeight"`
	LineHeight           float64 `json:"lineHeight"`
	LetterSpacing        float64 `json:"letterSpacing"`
	TextColor            string  `json:"textColor"`
	HorizontalAlign      string  `json:"horizontalAlign"`
	VerticalAlign        string  `json:"verticalAlign"`
	OutlineWidth         int     `json:"outlineWidth"`
	OutlineColor         string  `json:"outlineColor"`
	ShadowEnabled        bool    `json:"shadowEnabled"`
	ShadowOffsetX        int     `json:"shadowOffsetX"`
	ShadowOffsetY        int     `json:"shadowOffsetY"`
	ShadowBlur           int     `json:"shadowBlur"`
	ShadowColor          string  `json:"shadowColor"`
}

type jsonAvatarProps struct {
	CornerRadius int    `json:"cornerRadius"`
	BorderColor  string `json:"borderColor"`
	BorderWidth  int    `json:"borderWidth"`
}

type jsonMessageFragmentsProps struct {
	FontFamily      string  `json:"fontFamily"`
	FontAssetID     string  `json:"fontAssetId,omitempty"`
	FontSize        int     `json:"fontSize"`
	FontWeight      int     `json:"fontWeight"`
	LineHeight      float64 `json:"lineHeight"`
	LetterSpacing   float64 `json:"letterSpacing"`
	TextColor       string  `json:"textColor"`
	HorizontalAlign string  `json:"horizontalAlign"`
	VerticalAlign   string  `json:"verticalAlign"`
	EmoteSize       int     `json:"emoteSize"`
}

type jsonBadgeListProps struct {
	MaxCount  int `json:"maxCount"`
	BadgeSize int `json:"badgeSize"`
	Gap       int `json:"gap"`
}

type jsonImageProps struct {
	AssetID string `json:"assetId"`
	Fit     string `json:"fit"`
	Alt     string `json:"alt,omitempty"`
}

type jsonVideoProps struct {
	AssetID string `json:"assetId"`
	Fit     string `json:"fit"`
	Loop    bool   `json:"loop"`
}

type jsonLayer struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Kind    string    `json:"kind"`
	Visible bool      `json:"visible"`
	Locked  bool      `json:"locked"`
	Order   int       `json:"order"`
	Frame   jsonFrame `json:"frame"`
	Opacity float64   `json:"opacity"`

	Shape            *jsonShapeProps            `json:"shape,omitempty"`
	Text             *jsonTextProps             `json:"text,omitempty"`
	PlatformIcon     *struct{}                  `json:"platformIcon,omitempty"`
	Avatar           *jsonAvatarProps           `json:"avatar,omitempty"`
	MessageFragments *jsonMessageFragmentsProps `json:"messageFragments,omitempty"`
	BadgeList        *jsonBadgeListProps        `json:"badgeList,omitempty"`
	Image            *jsonImageProps            `json:"image,omitempty"`
	Video            *jsonVideoProps            `json:"video,omitempty"`

	EntryAnimation      string `json:"entryAnimation"`
	ExitAnimation       string `json:"exitAnimation"`
	AnimationDurationMS int    `json:"animationDurationMs"`
}

func documentToJSONMirror(doc Document) jsonDocument {
	layers := make([]jsonLayer, 0, len(doc.Layers))
	for _, l := range doc.Layers {
		jl := jsonLayer{
			ID: l.ID, Name: l.Name, Kind: string(l.Kind), Visible: l.Visible, Locked: l.Locked, Order: l.Order,
			Frame:          jsonFrame{X: l.Frame.X, Y: l.Frame.Y, Width: l.Frame.Width, Height: l.Frame.Height},
			Opacity:        l.Opacity,
			EntryAnimation: string(l.EntryAnimation), ExitAnimation: string(l.ExitAnimation), AnimationDurationMS: l.AnimationDurationMS,
		}
		if l.Shape != nil {
			jl.Shape = &jsonShapeProps{
				Kind: string(l.Shape.Kind), Fill: l.Shape.Fill, BorderColor: l.Shape.BorderColor,
				BorderWidth: l.Shape.BorderWidth, CornerRadius: l.Shape.CornerRadius,
			}
		}
		if l.Text != nil {
			jl.Text = &jsonTextProps{
				Binding: string(l.Text.Binding), StaticText: l.Text.StaticText, MissingValueBehavior: string(l.Text.MissingValueBehavior),
				FontFamily: string(l.Text.FontFamily), FontAssetID: l.Text.FontAssetID, FontSize: l.Text.FontSize, FontWeight: l.Text.FontWeight,
				LineHeight: l.Text.LineHeight, LetterSpacing: l.Text.LetterSpacing, TextColor: l.Text.TextColor,
				HorizontalAlign: string(l.Text.HorizontalAlign), VerticalAlign: string(l.Text.VerticalAlign),
				OutlineWidth: l.Text.OutlineWidth, OutlineColor: l.Text.OutlineColor,
				ShadowEnabled: l.Text.ShadowEnabled, ShadowOffsetX: l.Text.ShadowOffsetX, ShadowOffsetY: l.Text.ShadowOffsetY,
				ShadowBlur: l.Text.ShadowBlur, ShadowColor: l.Text.ShadowColor,
			}
		}
		if l.PlatformIcon != nil {
			jl.PlatformIcon = &struct{}{}
		}
		if l.Avatar != nil {
			jl.Avatar = &jsonAvatarProps{CornerRadius: l.Avatar.CornerRadius, BorderColor: l.Avatar.BorderColor, BorderWidth: l.Avatar.BorderWidth}
		}
		if l.MessageFragments != nil {
			jl.MessageFragments = &jsonMessageFragmentsProps{
				FontFamily: string(l.MessageFragments.FontFamily), FontAssetID: l.MessageFragments.FontAssetID, FontSize: l.MessageFragments.FontSize, FontWeight: l.MessageFragments.FontWeight,
				LineHeight: l.MessageFragments.LineHeight, LetterSpacing: l.MessageFragments.LetterSpacing, TextColor: l.MessageFragments.TextColor,
				HorizontalAlign: string(l.MessageFragments.HorizontalAlign), VerticalAlign: string(l.MessageFragments.VerticalAlign),
				EmoteSize: l.MessageFragments.EmoteSize,
			}
		}
		if l.BadgeList != nil {
			jl.BadgeList = &jsonBadgeListProps{MaxCount: l.BadgeList.MaxCount, BadgeSize: l.BadgeList.BadgeSize, Gap: l.BadgeList.Gap}
		}
		if l.Image != nil {
			jl.Image = &jsonImageProps{AssetID: l.Image.AssetID, Fit: string(l.Image.Fit), Alt: l.Image.Alt}
		}
		if l.Video != nil {
			jl.Video = &jsonVideoProps{AssetID: l.Video.AssetID, Fit: string(l.Video.Fit), Loop: l.Video.Loop}
		}
		layers = append(layers, jl)
	}
	return jsonDocument{
		Version: doc.Version,
		Canvas:  jsonCanvas{Width: doc.Canvas.Width, Height: doc.Canvas.Height, Transparent: doc.Canvas.Transparent},
		Layers:  layers,
	}
}

func documentFromJSONMirror(jd jsonDocument) Document {
	layers := make([]Layer, 0, len(jd.Layers))
	for _, jl := range jd.Layers {
		l := Layer{
			ID: jl.ID, Name: jl.Name, Kind: LayerKind(jl.Kind), Visible: jl.Visible, Locked: jl.Locked, Order: jl.Order,
			Frame:          Frame{X: jl.Frame.X, Y: jl.Frame.Y, Width: jl.Frame.Width, Height: jl.Frame.Height},
			Opacity:        jl.Opacity,
			EntryAnimation: Animation(jl.EntryAnimation), ExitAnimation: Animation(jl.ExitAnimation),
			AnimationDurationMS: jl.AnimationDurationMS,
		}
		if jl.Shape != nil {
			l.Shape = &ShapeProps{
				Kind: ShapeKind(jl.Shape.Kind), Fill: jl.Shape.Fill, BorderColor: jl.Shape.BorderColor,
				BorderWidth: jl.Shape.BorderWidth, CornerRadius: jl.Shape.CornerRadius,
			}
		}
		if jl.Text != nil {
			l.Text = &TextProps{
				Binding: TextBinding(jl.Text.Binding), StaticText: jl.Text.StaticText,
				MissingValueBehavior: MissingValueBehavior(jl.Text.MissingValueBehavior),
				FontFamily:           FontFamily(jl.Text.FontFamily), FontAssetID: jl.Text.FontAssetID, FontSize: jl.Text.FontSize, FontWeight: jl.Text.FontWeight,
				LineHeight: jl.Text.LineHeight, LetterSpacing: jl.Text.LetterSpacing, TextColor: jl.Text.TextColor,
				HorizontalAlign: HorizontalAlign(jl.Text.HorizontalAlign), VerticalAlign: VerticalAlign(jl.Text.VerticalAlign),
				OutlineWidth: jl.Text.OutlineWidth, OutlineColor: jl.Text.OutlineColor,
				ShadowEnabled: jl.Text.ShadowEnabled, ShadowOffsetX: jl.Text.ShadowOffsetX, ShadowOffsetY: jl.Text.ShadowOffsetY,
				ShadowBlur: jl.Text.ShadowBlur, ShadowColor: jl.Text.ShadowColor,
			}
		}
		if jl.PlatformIcon != nil {
			l.PlatformIcon = &PlatformIconProps{}
		}
		if jl.Avatar != nil {
			l.Avatar = &AvatarProps{CornerRadius: jl.Avatar.CornerRadius, BorderColor: jl.Avatar.BorderColor, BorderWidth: jl.Avatar.BorderWidth}
		}
		if jl.MessageFragments != nil {
			l.MessageFragments = &MessageFragmentsProps{
				FontFamily: FontFamily(jl.MessageFragments.FontFamily), FontAssetID: jl.MessageFragments.FontAssetID, FontSize: jl.MessageFragments.FontSize, FontWeight: jl.MessageFragments.FontWeight,
				LineHeight: jl.MessageFragments.LineHeight, LetterSpacing: jl.MessageFragments.LetterSpacing, TextColor: jl.MessageFragments.TextColor,
				HorizontalAlign: HorizontalAlign(jl.MessageFragments.HorizontalAlign), VerticalAlign: VerticalAlign(jl.MessageFragments.VerticalAlign),
				EmoteSize: jl.MessageFragments.EmoteSize,
			}
		}
		if jl.BadgeList != nil {
			l.BadgeList = &BadgeListProps{MaxCount: jl.BadgeList.MaxCount, BadgeSize: jl.BadgeList.BadgeSize, Gap: jl.BadgeList.Gap}
		}
		if jl.Image != nil {
			l.Image = &ImageProps{AssetID: jl.Image.AssetID, Fit: ImageFit(jl.Image.Fit), Alt: jl.Image.Alt}
		}
		if jl.Video != nil {
			l.Video = &VideoProps{AssetID: jl.Video.AssetID, Fit: ImageFit(jl.Video.Fit), Loop: jl.Video.Loop}
		}
		layers = append(layers, l)
	}
	return Document{
		Version: jd.Version,
		Canvas:  Canvas{Width: jd.Canvas.Width, Height: jd.Canvas.Height, Transparent: jd.Canvas.Transparent},
		Layers:  layers,
	}
}

// MarshalDocumentJSON renders doc using the project's one canonical
// Document wire shape.
func MarshalDocumentJSON(doc Document) ([]byte, error) {
	return json.Marshal(documentToJSONMirror(doc))
}

// UnmarshalDocumentJSON parses data (produced by MarshalDocumentJSON, or
// any client following the same wire shape) back into a Document. The
// result is not yet validated or migrated - callers must still run
// MigrateToCurrentVersion and Validate before trusting it.
func UnmarshalDocumentJSON(data []byte) (Document, error) {
	var jd jsonDocument
	if err := json.Unmarshal(data, &jd); err != nil {
		return Document{}, err
	}
	return documentFromJSONMirror(jd), nil
}
