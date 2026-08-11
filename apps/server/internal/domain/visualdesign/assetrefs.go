package visualdesign

import "fmt"

// AssetResolverFunc looks up a managed asset's own kind by local id -
// existence and kind alone, nothing else, the only two things a
// visual-design consumer needs before persisting a document containing
// a Stage 14B asset reference (docs/visual-template-packages.md §12's
// "two validation layers": this package's own structural format check,
// plus this existence/kind check the owning service supplies). This
// package never imports internal/domain/visualasset directly - the
// owning Manager (internal/alerts, internal/chatoverlay) supplies a
// small closure backed by whatever concrete visualasset.Service it was
// constructed with, exactly like OwnerBindingCheck already does for
// text-binding compatibility in the sibling visualtemplate package.
type AssetResolverFunc func(assetID string) (kind string, found bool)

// ValidateAssetReferences checks that every asset reference doc's own
// layers carry actually exists and is the expected kind for the field
// referencing it, using resolve. resolve == nil is only valid when doc
// carries no asset reference at all - a resolver-less caller facing a
// real reference gets ErrAssetMissing, never a silent pass.
func ValidateAssetReferences(doc Document, resolve AssetResolverFunc) error {
	for _, l := range doc.Layers {
		if l.Image != nil && l.Image.AssetID != "" {
			if err := checkAssetKind(resolve, l.Image.AssetID, "image"); err != nil {
				return err
			}
		}
		if l.Video != nil && l.Video.AssetID != "" {
			if err := checkAssetKind(resolve, l.Video.AssetID, "video"); err != nil {
				return err
			}
		}
		if l.Text != nil && l.Text.FontAssetID != "" {
			if err := checkAssetKind(resolve, l.Text.FontAssetID, "font"); err != nil {
				return err
			}
		}
		if l.MessageFragments != nil && l.MessageFragments.FontAssetID != "" {
			if err := checkAssetKind(resolve, l.MessageFragments.FontAssetID, "font"); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkAssetKind(resolve AssetResolverFunc, id, want string) error {
	if resolve == nil {
		return fmt.Errorf("%w: %q", ErrAssetMissing, id)
	}
	kind, found := resolve(id)
	if !found {
		return fmt.Errorf("%w: %q", ErrAssetMissing, id)
	}
	if kind != want {
		return fmt.Errorf("%w: %q is a %q asset, expected %q", ErrAssetKindMismatch, id, kind, want)
	}
	return nil
}

// AssetReferences returns every managed-asset reference doc's own
// layers carry - image/video asset ids, plus any optional custom-font
// asset id on a text-capable layer (Stage 14B, docs/visual-template-
// packages.md §12). Exported so the owning domain packages
// (internal/domain/alerts, internal/domain/chatoverlay) can validate
// existence and maintain visualasset's own reference-tracking tables on
// every save, without either package needing to walk Layer's own
// payload union itself - this package is the one place that union is
// defined. Deduplicated; order is not meaningful.
func (d Document) AssetReferences() []string {
	seen := make(map[string]bool)
	var refs []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			refs = append(refs, id)
		}
	}
	for _, l := range d.Layers {
		if l.Image != nil {
			add(l.Image.AssetID)
		}
		if l.Video != nil {
			add(l.Video.AssetID)
		}
		if l.Text != nil {
			add(l.Text.FontAssetID)
		}
		if l.MessageFragments != nil {
			add(l.MessageFragments.FontAssetID)
		}
	}
	return refs
}
