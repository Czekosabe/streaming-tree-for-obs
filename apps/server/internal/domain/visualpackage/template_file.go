package visualpackage

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// templateFileMirror is template.json's own wire shape - deliberately
// identical to the Stage 14A asset-free JSON template-interchange file
// (visualtemplate.Format/schema B), plus asset references the embedded
// document may now carry (Stage 14B, docs/visual-template-packages.md
// §5: template.json is "the exact same portable template file shape",
// simply embedded inside the archive alongside manifest.json/assets/
// rather than standing alone). VisualDesign is kept raw so it can be
// decoded through visualdesign.UnmarshalDocumentJSON, the one shared
// wire mirror, rather than a second private one.
type templateFileMirror struct {
	Format        string          `json:"format"`
	SchemaVersion int             `json:"schemaVersion"`
	Target        string          `json:"target"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Author        string          `json:"author"`
	License       string          `json:"license"`
	VisualDesign  json.RawMessage `json:"visualDesign"`
}

// parseTemplateFile decodes and structurally checks template.json,
// returning the still-package-local (pkgasset_-referencing) document
// exactly as embedded - the caller (Import) is responsible for
// migrating, rewriting asset references, and final validation.
func parseTemplateFile(raw []byte) (target visualtemplate.Target, name, description, author, license string, doc visualdesign.Document, err error) {
	var m templateFileMirror
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if decErr := dec.Decode(&m); decErr != nil {
		return "", "", "", "", "", visualdesign.Document{}, fmt.Errorf("%w: template.json: %v", ErrManifestInvalid, decErr)
	}
	if m.Format != visualtemplate.Format {
		return "", "", "", "", "", visualdesign.Document{}, fmt.Errorf("%w: template.json format %q is not %q", ErrManifestInvalid, m.Format, visualtemplate.Format)
	}
	if m.SchemaVersion != visualtemplate.CurrentTemplateSchemaVersion {
		return "", "", "", "", "", visualdesign.Document{}, fmt.Errorf("%w: template.json schema version %d is not supported", ErrVersionUnsupported, m.SchemaVersion)
	}
	parsedDoc, docErr := visualdesign.UnmarshalDocumentJSON(m.VisualDesign)
	if docErr != nil {
		return "", "", "", "", "", visualdesign.Document{}, fmt.Errorf("%w: template.json visualDesign: %v", ErrManifestInvalid, docErr)
	}
	if parsedDoc.Version != visualdesign.Version1 && parsedDoc.Version != visualdesign.Version2 && parsedDoc.Version != visualdesign.Version3 {
		return "", "", "", "", "", visualdesign.Document{}, fmt.Errorf("%w: embedded document version %d is not supported", ErrVersionUnsupported, parsedDoc.Version)
	}
	return visualtemplate.Target(m.Target), m.Name, m.Description, m.Author, m.License, parsedDoc, nil
}

// marshalTemplateFile renders template.json's own wire shape for
// export - the mirror image of parseTemplateFile.
func marshalTemplateFile(target visualtemplate.Target, name, description, author, license string, doc visualdesign.Document) ([]byte, error) {
	docJSON, err := visualdesign.MarshalDocumentJSON(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal template document: %w", err)
	}
	m := templateFileMirror{
		Format: visualtemplate.Format, SchemaVersion: visualtemplate.CurrentTemplateSchemaVersion,
		Target: string(target), Name: name, Description: description, Author: author, License: license,
		VisualDesign: docJSON,
	}
	return json.Marshal(m)
}

// collectAssetRefs returns every managed-asset reference doc's own
// layers carry (image/video asset ids, optional font asset ids) -
// package-local (pkgasset_...) before rewriteAssetRefs runs, local
// (asset_...) after.
func collectAssetRefs(doc visualdesign.Document) map[string]bool {
	refs := make(map[string]bool)
	for _, l := range doc.Layers {
		if l.Image != nil && l.Image.AssetID != "" {
			refs[l.Image.AssetID] = true
		}
		if l.Video != nil && l.Video.AssetID != "" {
			refs[l.Video.AssetID] = true
		}
		if l.Text != nil && l.Text.FontAssetID != "" {
			refs[l.Text.FontAssetID] = true
		}
		if l.MessageFragments != nil && l.MessageFragments.FontAssetID != "" {
			refs[l.MessageFragments.FontAssetID] = true
		}
	}
	return refs
}

// rewriteAssetRefs replaces every asset reference doc's layers carry
// using idMap (docs/visual-template-packages.md §6: "rewrite package-
// local references in the visual document to local IDs"). A reference
// with no entry in idMap is left untouched - it then fails
// visualdesign.Validate's own assetRefPattern check downstream (a
// package-local pkgasset_ id is never itself a valid asset_ reference),
// which is the deliberate fallback enforcement for an unresolved
// reference, on top of the explicit pre-check Import already performs.
func rewriteAssetRefs(doc visualdesign.Document, idMap map[string]string) visualdesign.Document {
	layers := make([]visualdesign.Layer, len(doc.Layers))
	copy(layers, doc.Layers)
	for i, l := range layers {
		if l.Image != nil {
			img := *l.Image
			if mapped, ok := idMap[img.AssetID]; ok {
				img.AssetID = mapped
			}
			layers[i].Image = &img
		}
		if l.Video != nil {
			vid := *l.Video
			if mapped, ok := idMap[vid.AssetID]; ok {
				vid.AssetID = mapped
			}
			layers[i].Video = &vid
		}
		if l.Text != nil {
			t := *l.Text
			if t.FontAssetID != "" {
				if mapped, ok := idMap[t.FontAssetID]; ok {
					t.FontAssetID = mapped
				}
			}
			layers[i].Text = &t
		}
		if l.MessageFragments != nil {
			mf := *l.MessageFragments
			if mf.FontAssetID != "" {
				if mapped, ok := idMap[mf.FontAssetID]; ok {
					mf.FontAssetID = mapped
				}
			}
			layers[i].MessageFragments = &mf
		}
	}
	doc.Layers = layers
	return doc
}
