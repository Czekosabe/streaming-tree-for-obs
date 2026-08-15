package visualpackage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/streaming-tree/server/internal/domain/audioasset"
	"github.com/streaming-tree/server/internal/domain/visualasset"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// Clock returns the current time - injected for deterministic tests,
// exactly like every other domain package's own Clock.
type Clock func() time.Time

// PreviewTTL is how long a staged preview session stays valid before it
// is treated as expired (docs/visual-template-packages.md §19/§44 -
// "TTL of roughly 10 minutes").
const PreviewTTL = 10 * time.Minute

// PreviewAsset is one staged package asset, described for a preview
// response - never a local managed asset (nothing is persisted at
// preview time).
type PreviewAsset struct {
	PackageAssetID string
	Kind           visualasset.Kind
	MediaType      visualasset.MediaType
	SizeBytes      int64
	DisplayName    string
	Author         string
	License        string
	Notice         string
	// LogicalName is the staged filename under the preview session's own
	// directory (docs/visual-template-packages.md §44) - an
	// application-generated name, safe to embed in a preview-scoped
	// asset-serving URL.
	LogicalName string
}

// PreviewSession is ReadArchive plus preview-only staging: the archive
// was fully validated, its assets were streamed into a temporary,
// token-scoped directory, and nothing was written to the normal
// template/asset tables (docs/visual-template-packages.md §43/§44).
// Document still carries package-local (pkgasset_...) asset references -
// the frontend renders it using a preview-scoped asset map keyed by
// PackageAssetID, exactly the same shape a local (asset_...) resolved
// asset map takes for a real saved design (docs/visual-template-
// packages.md §42).
type PreviewSession struct {
	Token       string
	Target      visualtemplate.Target
	Name        string
	Description string
	Author      string
	License     string
	Document    visualdesign.Document
	Assets      []PreviewAsset
	// AlertAudio describes the package's own optional alert-audio
	// preset for display purposes only (docs/alert-audio.md §12:
	// "package preview identifies audio") - nil when the package
	// carries none. Preview never stages or plays the sound bytes
	// themselves; only ImportPreview's own structural validation has
	// already run against them by this point.
	AlertAudio *PreviewAlertAudio
	ExpiresAt  time.Time
}

// PreviewAlertAudio is a preview-only, read-only projection of a v2
// package's own alertAudio configuration - never persisted, never
// itself proof a later Import call reuses (mirrors PreviewSession's own
// doc comment).
type PreviewAlertAudio struct {
	SoundEnabled     bool
	SoundDisplayName string
	SoundDurationMS  int64
	TTSEnabled       bool
	TTSTemplate      string
}

// Service orchestrates ReadArchive/WriteArchive against the managed
// asset and template services - the one place a portable archive
// actually becomes local rows, or local rows become a portable archive
// (docs/visual-template-packages.md §20/§43).
type Service struct {
	assets      *visualasset.Service
	audioAssets *audioasset.Service
	templates   *visualtemplate.Service
	now         Clock
}

func NewService(assets *visualasset.Service, audioAssets *audioasset.Service, templates *visualtemplate.Service, now Clock) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{assets: assets, audioAssets: audioAssets, templates: templates, now: now}
}

func newPreviewToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate preview token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// crossCheckReferences enforces docs/visual-template-packages.md §58's
// two-directional rule: every asset reference the document actually
// uses must exist in the manifest (ErrAssetMissing), and every manifest
// asset must actually be referenced by the document (ErrAssetUnreferenced
// - "a package should contain exactly the bytes it needs").
func crossCheckReferences(doc visualdesign.Document, manifest Manifest) error {
	referenced := collectAssetRefs(doc)
	manifestIDs := make(map[string]bool, len(manifest.Assets))
	for _, a := range manifest.Assets {
		manifestIDs[a.ID] = true
	}
	for ref := range referenced {
		if strings.HasPrefix(ref, "pkgasset_") && !manifestIDs[ref] {
			return fmt.Errorf("%w: document references asset %q, which is not in the manifest", ErrAssetMissing, ref)
		}
	}
	for id := range manifestIDs {
		if !referenced[id] {
			return fmt.Errorf("%w: manifest asset %q is not referenced by the template document", ErrAssetUnreferenced, id)
		}
	}
	return nil
}

// placeholderValidate runs visualdesign.Validate against a copy of doc
// where every package-local (pkgasset_...) reference has been rewritten
// to a syntactically-valid but not-yet-real local-shaped id - a
// deliberate, narrow trick that lets preview enforce every real
// structural/bounds rule (frame geometry, color, font size, enum
// values, and so on) without needing actual local asset rows to exist
// yet (docs/visual-template-packages.md §43: preview persists nothing).
// The placeholder mapping is never returned to a caller - only used to
// satisfy Validate's own asset-reference format check for this one call.
func placeholderValidate(doc visualdesign.Document) error {
	placeholders := make(map[string]string)
	for ref := range collectAssetRefs(doc) {
		if strings.HasPrefix(ref, "pkgasset_") {
			placeholders[ref] = visualasset.AssetIDPrefix + strings.TrimPrefix(ref, "pkgasset_")
		}
	}
	return visualdesign.Validate(rewriteAssetRefs(doc, placeholders))
}

// ImportPreview fully validates raw as a `.streaming-tree-template`
// package and stages its verified asset bytes under a fresh, random
// preview token - it never touches the normal template/asset tables
// (docs/visual-template-packages.md §43/§44).
func (s *Service) ImportPreview(ctx context.Context, raw []byte) (PreviewSession, error) {
	validated, err := ReadArchive(raw)
	if err != nil {
		return PreviewSession{}, err
	}
	target, name, description, author, license, doc, err := parseTemplateFile(validated.TemplateJSON)
	if err != nil {
		return PreviewSession{}, err
	}
	// docs/alert-audio.md §10.2: a chat-target package carrying an
	// alertAudio object is rejected outright, before any asset (visual
	// or audio) is even staged.
	if validated.Manifest.AlertAudio != nil && target != visualtemplate.TargetAlert {
		return PreviewSession{}, ErrAudioTargetInvalid
	}
	doc = visualdesign.MigrateToCurrentVersion(doc)
	if err := crossCheckReferences(doc, validated.Manifest); err != nil {
		return PreviewSession{}, err
	}
	if err := placeholderValidate(doc); err != nil {
		return PreviewSession{}, err
	}

	token, err := newPreviewToken()
	if err != nil {
		return PreviewSession{}, err
	}

	// Stage every asset's already-verified bytes (ReadArchive already
	// read them fully and checked every bound) under the fresh preview
	// token - never re-derived from anything but this call's own bytes.
	previewAssets := make([]PreviewAsset, 0, len(validated.Assets))
	for _, va := range validated.Assets {
		logicalName := va.Manifest.ID
		if _, _, _, err := s.assets.WritePreviewAsset(token, logicalName, bytes.NewReader(va.Data), int64(len(va.Data))); err != nil {
			_ = s.assets.RemovePreview(token)
			return PreviewSession{}, err
		}
		previewAssets = append(previewAssets, PreviewAsset{
			PackageAssetID: va.Manifest.ID, Kind: va.Kind, MediaType: va.MediaType, SizeBytes: va.Manifest.SizeBytes,
			DisplayName: va.Manifest.DisplayName, Author: va.Manifest.Author, License: va.Manifest.License, Notice: va.Manifest.Notice,
			LogicalName: logicalName,
		})
	}

	var previewAudio *PreviewAlertAudio
	if ma := validated.Manifest.AlertAudio; ma != nil {
		pa := &PreviewAlertAudio{SoundEnabled: ma.SoundEnabled, TTSEnabled: ma.TTSEnabled, TTSTemplate: ma.TTSTemplate}
		if ma.SoundEnabled {
			for _, va := range validated.AudioAssets {
				if va.Manifest.ID == ma.SoundAssetID {
					pa.SoundDisplayName = va.Manifest.DisplayName
					pa.SoundDurationMS = va.Manifest.DurationMS
					break
				}
			}
		}
		previewAudio = pa
	}

	now := s.now()
	return PreviewSession{
		Token: token, Target: target, Name: name, Description: description, Author: author, License: license,
		Document: doc, Assets: previewAssets, AlertAudio: previewAudio, ExpiresAt: now.Add(PreviewTTL),
	}, nil
}

// CancelPreview removes a preview session's staged bytes immediately -
// used on explicit cancel (docs/visual-template-packages.md §44).
func (s *Service) CancelPreview(token string) error {
	return s.assets.RemovePreview(token)
}

// Import fully re-validates raw from scratch - it never trusts a prior
// ImportPreview call's own result as proof (docs/visual-template-
// packages.md §19 step 6/§71: "actual import does not trust preview").
// A successful import creates exactly: one new user template, one
// managed logical asset per package asset (deduplicated at the blob
// level), and template-asset reference rows - never an owner design,
// never a public presentation event (docs/visual-template-packages.md
// §20/§45).
func (s *Service) Import(ctx context.Context, raw []byte) (visualtemplate.Template, error) {
	validated, err := ReadArchive(raw)
	if err != nil {
		return visualtemplate.Template{}, err
	}
	target, name, description, author, license, doc, err := parseTemplateFile(validated.TemplateJSON)
	if err != nil {
		return visualtemplate.Template{}, err
	}
	// docs/alert-audio.md §10.2: a chat-target package carrying an
	// alertAudio object is rejected outright, before any asset (visual
	// or audio) is even staged/uploaded.
	if validated.Manifest.AlertAudio != nil && target != visualtemplate.TargetAlert {
		return visualtemplate.Template{}, ErrAudioTargetInvalid
	}
	doc = visualdesign.MigrateToCurrentVersion(doc)
	if err := crossCheckReferences(doc, validated.Manifest); err != nil {
		return visualtemplate.Template{}, err
	}

	idMap := make(map[string]string, len(validated.Assets))
	createdAssetIDs := make([]string, 0, len(validated.Assets))
	for _, va := range validated.Assets {
		asset, err := s.assets.Upload(ctx, va.Data, "", string(va.MediaType),
			va.Manifest.DisplayName, va.Manifest.Author, va.Manifest.License, va.Manifest.Notice, visualasset.SourcePackage)
		if err != nil {
			return visualtemplate.Template{}, err
		}
		idMap[va.Manifest.ID] = asset.ID
		createdAssetIDs = append(createdAssetIDs, asset.ID)
	}

	doc = rewriteAssetRefs(doc, idMap)

	// docs/alert-audio.md §10.4: fresh local audioasset_ IDs for every
	// accepted audioAssets entry, then the preset's own soundAssetId is
	// rewritten to the real local ID before persistence - a package-
	// supplied pkgaudio_ ID is never written into
	// alert_template_audio_asset_refs/audioasset tables.
	var audioPreset *visualtemplate.RuleAudioPreset
	if ma := validated.Manifest.AlertAudio; ma != nil {
		if s.audioAssets == nil {
			return visualtemplate.Template{}, fmt.Errorf("%w: package audio import is not available", ErrAssetUnsupported)
		}
		audioIDMap := make(map[string]string, len(validated.AudioAssets))
		for _, va := range validated.AudioAssets {
			asset, err := s.audioAssets.Upload(ctx, va.Data, "", string(va.MediaType), va.Manifest.DisplayName, audioasset.SourcePackage)
			if err != nil {
				return visualtemplate.Template{}, err
			}
			audioIDMap[va.Manifest.ID] = asset.ID
		}
		preset := visualtemplate.RuleAudioPreset{
			SoundEnabled: ma.SoundEnabled, SoundVolume: ma.SoundVolume,
			TTSEnabled: ma.TTSEnabled, TTSTemplate: ma.TTSTemplate, TTSVolume: ma.TTSVolume,
		}
		if ma.SoundAssetID != "" {
			mapped, ok := audioIDMap[ma.SoundAssetID]
			if !ok {
				return visualtemplate.Template{}, fmt.Errorf("%w: alertAudio references an unmapped audio asset", ErrAssetMissing)
			}
			preset.SoundAssetID = mapped
		}
		audioPreset = &preset
	}

	tmpl, err := s.templates.CreatePackaged(ctx, target, name, description, author, license, doc, audioPreset)
	if err != nil {
		return visualtemplate.Template{}, err
	}
	if err := s.assets.SetTemplateAssetRefs(ctx, tmpl.ID, createdAssetIDs); err != nil {
		return visualtemplate.Template{}, err
	}
	return tmpl, nil
}

// mediaTypeExtension maps an accepted asset media type to a safe,
// bounded ASCII extension for its exported archive entry name (docs/
// visual-template-packages.md §7's own segment grammar).
var mediaTypeExtension = map[visualasset.MediaType]string{
	visualasset.MediaPNG:   "png",
	visualasset.MediaJPEG:  "jpg",
	visualasset.MediaGIF:   "gif",
	visualasset.MediaWebP:  "webp",
	visualasset.MediaWebM:  "webm",
	visualasset.MediaMP4:   "mp4",
	visualasset.MediaWOFF2: "woff2",
}

// ExportTemplate builds a complete, valid package archive for an
// existing template - asset-free if the template's document references
// no managed asset, otherwise containing exactly the transitively
// referenced assets (docs/visual-template-packages.md §20/§50/§51).
// Local managed asset ids are remapped to deterministic, sorted
// package-local ids; the stored template and its assets are never
// mutated by an export.
func (s *Service) ExportTemplate(ctx context.Context, templateID string) ([]byte, error) {
	tmpl, err := s.templates.Export(ctx, templateID)
	if err != nil {
		return nil, err
	}

	refs := collectAssetRefs(tmpl.Document)
	localIDs := make([]string, 0, len(refs))
	for id := range refs {
		localIDs = append(localIDs, id)
	}
	sort.Strings(localIDs)

	idMap := make(map[string]string, len(localIDs))
	manifestAssets := make([]ManifestAsset, 0, len(localIDs))
	exportAssets := make([]ExportAsset, 0, len(localIDs))
	for i, localID := range localIDs {
		asset, err := s.assets.Get(ctx, localID)
		if err != nil {
			return nil, err
		}
		if asset.Blob == nil {
			return nil, fmt.Errorf("%w: asset %q has no resolvable blob", ErrAssetMissing, localID)
		}
		data, err := s.readBlobBytes(asset.Blob.SHA256)
		if err != nil {
			return nil, err
		}
		ext, ok := mediaTypeExtension[asset.Blob.MediaType]
		if !ok {
			return nil, fmt.Errorf("%w: asset %q has an unsupported media type %q", ErrAssetUnsupported, localID, asset.Blob.MediaType)
		}
		pkgID := fmt.Sprintf("pkgasset_%04d", i+1)
		idMap[localID] = pkgID
		path := fmt.Sprintf("assets/%s.%s", pkgID, ext)

		ma := ManifestAsset{
			ID: pkgID, Path: path, Kind: string(asset.Kind), MediaType: string(asset.Blob.MediaType),
			SHA256: asset.Blob.SHA256, SizeBytes: asset.Blob.ByteSize,
			DisplayName: asset.DisplayName, Author: asset.Author, License: asset.License, Notice: asset.Notice,
		}
		manifestAssets = append(manifestAssets, ma)
		exportAssets = append(exportAssets, ExportAsset{Manifest: ma, Data: data})
	}

	doc := rewriteAssetRefs(tmpl.Document, idMap)
	templateFileBytes, err := marshalTemplateFile(tmpl.Target, tmpl.Name, tmpl.Description, tmpl.Author, tmpl.License, doc)
	if err != nil {
		return nil, err
	}

	// docs/alert-audio.md §10.1: schemaVersion stays 1 for a purely
	// visual template (never silently upgrading a visual-only export
	// format) - v2, and the alertAudio/audioAssets manifest objects, are
	// written only when the template actually carries a configured
	// preset (sound or TTS or both).
	schemaVersion := ManifestSchemaVersionV1
	var manifestAudio *ManifestAlertAudio
	var manifestAudioAssets []ManifestAudioAsset
	var exportAudioAssets []ExportAudioAsset
	if tmpl.AlertAudio.HasAudio() {
		schemaVersion = ManifestSchemaVersionV2
		ma := &ManifestAlertAudio{
			SoundEnabled: tmpl.AlertAudio.SoundEnabled, SoundVolume: tmpl.AlertAudio.SoundVolume,
			TTSEnabled: tmpl.AlertAudio.TTSEnabled, TTSTemplate: tmpl.AlertAudio.TTSTemplate, TTSVolume: tmpl.AlertAudio.TTSVolume,
		}
		if tmpl.AlertAudio.SoundEnabled {
			if s.audioAssets == nil {
				return nil, fmt.Errorf("%w: audio asset %q has no resolvable blob", ErrAssetMissing, tmpl.AlertAudio.SoundAssetID)
			}
			asset, err := s.audioAssets.Get(ctx, tmpl.AlertAudio.SoundAssetID)
			if err != nil {
				return nil, err
			}
			if asset.Blob == nil {
				return nil, fmt.Errorf("%w: audio asset %q has no resolvable blob", ErrAssetMissing, tmpl.AlertAudio.SoundAssetID)
			}
			data, err := s.readAudioBlobBytes(asset.Blob.SHA256)
			if err != nil {
				return nil, err
			}
			pkgID := "pkgaudio_0001"
			ma.SoundAssetID = pkgID
			path := fmt.Sprintf("audio/%s.wav", pkgID)
			maa := ManifestAudioAsset{
				ID: pkgID, Path: path, MediaType: string(asset.Blob.MediaType),
				SHA256: asset.Blob.SHA256, SizeBytes: asset.Blob.ByteSize, DurationMS: asset.Blob.DurationMS,
				DisplayName: asset.DisplayName,
			}
			manifestAudioAssets = append(manifestAudioAssets, maa)
			exportAudioAssets = append(exportAudioAssets, ExportAudioAsset{Manifest: maa, Data: data})
		}
		manifestAudio = ma
	}

	manifest := Manifest{
		Format: Format, SchemaVersion: schemaVersion, TemplatePath: TemplatePath, Assets: manifestAssets,
		AlertAudio: manifestAudio, AudioAssets: manifestAudioAssets,
	}

	var buf bytes.Buffer
	if err := WriteArchive(&buf, manifest, templateFileBytes, exportAssets, exportAudioAssets); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *Service) readBlobBytes(sha256Hex string) ([]byte, error) {
	f, err := s.assets.OpenBlob(sha256Hex)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (s *Service) readAudioBlobBytes(sha256Hex string) ([]byte, error) {
	f, err := s.audioAssets.OpenBlob(sha256Hex)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
