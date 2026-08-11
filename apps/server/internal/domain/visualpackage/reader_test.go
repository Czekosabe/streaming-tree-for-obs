package visualpackage

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestReadArchive_ValidPackageAccepted(t *testing.T) {
	data := validPackageZip(t)
	validated, err := ReadArchive(data)
	if err != nil {
		t.Fatalf("ReadArchive() returned an error for a valid package: %v", err)
	}
	if len(validated.Assets) != 1 {
		t.Fatalf("expected 1 validated asset, got %d", len(validated.Assets))
	}
	if validated.Assets[0].Manifest.ID != "pkgasset_0001" {
		t.Errorf("unexpected asset id %q", validated.Assets[0].Manifest.ID)
	}
}

func TestReadArchive_EmptyArchiveRejected(t *testing.T) {
	data := buildZipCustom(t, nil)
	if _, err := ReadArchive(data); err == nil {
		t.Fatal("expected an error for an empty archive")
	}
}

func TestReadArchive_MissingManifestRejected(t *testing.T) {
	manifest, template, png := validPackageParts(t)
	_ = manifest
	data := buildZipCustom(t, []zipEntry{
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: png},
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("expected ErrManifestInvalid, got %v", err)
	}
}

func TestReadArchive_MissingTemplateRejected(t *testing.T) {
	manifest, _, png := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: "assets/pkgasset_0001.png", data: png},
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("expected ErrManifestInvalid, got %v", err)
	}
}

func TestReadArchive_ExtraRootFileRejected(t *testing.T) {
	manifest, template, png := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: png},
		{name: "evil.txt", data: []byte("not a declared asset")},
	})
	// "evil.txt" is outside the allowed root structure entirely (not
	// manifest.json/template.json/assets/<file>) - rejected by the path
	// grammar itself, even before any manifest cross-check runs.
	if _, err := ReadArchive(data); !errors.Is(err, ErrEntryInvalid) {
		t.Fatalf("expected ErrEntryInvalid for an extra root file, got %v", err)
	}
}

func TestReadArchive_UnreferencedAssetInAssetsDirRejected(t *testing.T) {
	manifest, template, png := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: png},
		{name: "assets/hidden_payload.png", data: png},
	})
	// "assets/hidden_payload.png" has a perfectly valid path shape but
	// is never declared in the manifest - a package must contain
	// exactly the bytes it needs (docs/visual-template-packages.md §58).
	if _, err := ReadArchive(data); !errors.Is(err, ErrAssetUnreferenced) {
		t.Fatalf("expected ErrAssetUnreferenced for an undeclared assets/ entry, got %v", err)
	}
}

func TestReadArchive_NestedArchiveRejected(t *testing.T) {
	manifest, template, _ := validPackageParts(t)
	var innerZip bytes.Buffer
	zw := zip.NewWriter(&innerZip)
	fw, _ := zw.Create("payload.txt")
	_, _ = fw.Write([]byte("nested"))
	_ = zw.Close()

	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: innerZip.Bytes()},
	})
	// The nested archive is accepted as *bytes* by ReadArchive's path/
	// mode checks (a ZIP file has no special path shape) but must still
	// be rejected once its content is checked against the manifest's
	// declared image signature/hash - it is not a valid PNG.
	if _, err := ReadArchive(data); err == nil {
		t.Fatal("expected an error for a nested-archive asset masquerading as an image")
	}
}

// --- path grammar matrix (docs/visual-template-packages.md §7/§66) ---

func TestReadArchive_PathGrammarMatrix(t *testing.T) {
	manifest, template, _ := validPackageParts(t)

	cases := []struct {
		name string
		path string
	}{
		{"absolute POSIX path", "/etc/passwd"},
		{"windows drive path", "C:/evil.png"},
		{"UNC-like path", "//server/share/evil.png"},
		{"backslash", "assets\\pkgasset_0001.png"},
		{"dot-dot traversal", "assets/../../../evil.png"},
		{"single dot segment", "assets/./pkgasset_0001.png"},
		{"empty segment", "assets//pkgasset_0001.png"},
		{"trailing dot", "assets/pkgasset_0001.png."},
		{"trailing space", "assets/pkgasset_0001.png "},
		{"reserved windows device name", "assets/NUL.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := buildZipCustom(t, []zipEntry{
				{name: "manifest.json", data: manifest},
				{name: TemplatePath, data: template},
				{name: tc.path, data: []byte("x")},
			})
			if _, err := ReadArchive(data); err == nil {
				t.Fatalf("expected ReadArchive to reject path %q", tc.path)
			}
		})
	}
}

func TestReadArchive_CaseInsensitiveDuplicatePathRejected(t *testing.T) {
	manifest, template, png := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: png},
		{name: "assets/PKGASSET_0001.png", data: png},
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrEntryInvalid) {
		t.Fatalf("expected ErrEntryInvalid for a case-insensitive duplicate path, got %v", err)
	}
}

func TestReadArchive_SymlinkEntryRejected(t *testing.T) {
	manifest, template, _ := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: []byte("/etc/passwd"), mode: fs.ModeSymlink | 0777},
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrEntryInvalid) {
		t.Fatalf("expected ErrEntryInvalid for a symlink entry, got %v", err)
	}
}

func TestReadArchive_DirectoryEntryOutsideGrammarRejected(t *testing.T) {
	manifest, template, png := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/", data: nil, mode: fs.ModeDir},
		{name: "assets/pkgasset_0001.png", data: png},
	})
	if _, err := ReadArchive(data); err == nil {
		t.Fatal("expected an error for an explicit directory entry")
	}
}

// --- manifest structural matrix ---

func TestReadArchive_DuplicateManifestAssetIDRejected(t *testing.T) {
	png := validPNGBytes(t)
	hash := sha256Hex(png)
	manifest := []byte(`{
		"format": "streaming-tree-template-package", "schemaVersion": 1, "templatePath": "template.json",
		"assets": [
			{"id": "pkgasset_0001", "path": "assets/a.png", "kind": "image", "mediaType": "image/png", "sha256": "` + hash + `", "sizeBytes": ` + itoa(len(png)) + `, "displayName": "", "author": "", "license": "", "notice": ""},
			{"id": "pkgasset_0001", "path": "assets/b.png", "kind": "image", "mediaType": "image/png", "sha256": "` + hash + `", "sizeBytes": ` + itoa(len(png)) + `, "displayName": "", "author": "", "license": "", "notice": ""}
		]
	}`)
	_, template, _ := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/a.png", data: png},
		{name: "assets/b.png", data: png},
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("expected ErrManifestInvalid for a duplicate manifest asset id, got %v", err)
	}
}

func TestReadArchive_AssetMissingFromArchiveRejected(t *testing.T) {
	manifest, template, _ := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		// the declared asset "assets/pkgasset_0001.png" is never written
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrAssetMissing) {
		t.Fatalf("expected ErrAssetMissing, got %v", err)
	}
}

// --- decompression/size bounds (docs/visual-template-packages.md §67) ---

func TestReadArchive_TooManyEntriesRejected(t *testing.T) {
	entries := []zipEntry{}
	for i := 0; i < MaxArchiveEntries+1; i++ {
		entries = append(entries, zipEntry{name: "assets/pad" + itoa(i) + ".png", data: []byte("x")})
	}
	data := buildZipCustom(t, entries)
	if _, err := ReadArchive(data); !errors.Is(err, ErrTooManyEntries) {
		t.Fatalf("expected ErrTooManyEntries, got %v", err)
	}
}

func TestReadArchive_TooManyAssetsRejected(t *testing.T) {
	png := validPNGBytes(t)
	hash := sha256Hex(png)
	var assetsJSON strings.Builder
	var entries []zipEntry
	entries = append(entries, zipEntry{}) // placeholder, replaced below
	entries = entries[:0]
	for i := 0; i < MaxAssets+1; i++ {
		id := "pkgasset_" + itoa(1000+i)
		path := "assets/" + id + ".png"
		if i > 0 {
			assetsJSON.WriteString(",")
		}
		assetsJSON.WriteString(`{"id":"` + id + `","path":"` + path + `","kind":"image","mediaType":"image/png","sha256":"` + hash + `","sizeBytes":` + itoa(len(png)) + `,"displayName":"","author":"","license":"","notice":""}`)
		entries = append(entries, zipEntry{name: path, data: png})
	}
	manifest := []byte(`{"format":"streaming-tree-template-package","schemaVersion":1,"templatePath":"template.json","assets":[` + assetsJSON.String() + `]}`)
	_, template, _ := validPackageParts(t)
	entries = append(entries, zipEntry{name: "manifest.json", data: manifest}, zipEntry{name: TemplatePath, data: template})
	data := buildZipCustom(t, entries)
	if _, err := ReadArchive(data); !errors.Is(err, ErrTooManyAssets) {
		t.Fatalf("expected ErrTooManyAssets, got %v", err)
	}
}

func TestReadArchive_OversizedPackageRejected(t *testing.T) {
	manifest, template, _ := validPackageParts(t)
	huge := bytes.Repeat([]byte{0}, int(MaxPackageBytes)+1024)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: huge},
	})
	if int64(len(data)) <= MaxPackageBytes {
		// Random-ish zero bytes compress extremely well; pad the raw
		// zip bytes themselves past the bound directly if compression
		// kept the archive under the limit.
		data = append(data, bytes.Repeat([]byte{0xFF}, int(MaxPackageBytes))...)
	}
	if _, err := ReadArchive(data); err == nil {
		t.Fatal("expected an error for an oversized package")
	}
}

// --- signature/type matrix (docs/visual-template-packages.md §68) ---

func TestReadArchive_AssetHashMismatchRejected(t *testing.T) {
	png := validPNGBytes(t)
	wrongHash := sha256Hex([]byte("not the same bytes"))
	manifest := []byte(`{"format":"streaming-tree-template-package","schemaVersion":1,"templatePath":"template.json","assets":[{"id":"pkgasset_0001","path":"assets/pkgasset_0001.png","kind":"image","mediaType":"image/png","sha256":"` + wrongHash + `","sizeBytes":` + itoa(len(png)) + `,"displayName":"","author":"","license":"","notice":""}]}`)
	_, template, _ := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: png},
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrAssetHashMismatch) {
		t.Fatalf("expected ErrAssetHashMismatch, got %v", err)
	}
}

func TestReadArchive_AssetContentNotActuallyAnImageRejected(t *testing.T) {
	fake := []byte("<svg onload=alert(1)></svg>")
	manifest := []byte(`{"format":"streaming-tree-template-package","schemaVersion":1,"templatePath":"template.json","assets":[{"id":"pkgasset_0001","path":"assets/pkgasset_0001.png","kind":"image","mediaType":"image/png","sha256":"` + sha256Hex(fake) + `","sizeBytes":` + itoa(len(fake)) + `,"displayName":"","author":"","license":"","notice":""}]}`)
	_, template, _ := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: fake},
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrAssetTypeMismatch) {
		t.Fatalf("expected ErrAssetTypeMismatch for SVG content masquerading as PNG, got %v", err)
	}
}

func TestReadArchive_ExecutableSignatureRejected(t *testing.T) {
	fake := []byte("MZ\x90\x00\x03\x00\x00\x00this is not an image, it looks like a PE header")
	manifest := []byte(`{"format":"streaming-tree-template-package","schemaVersion":1,"templatePath":"template.json","assets":[{"id":"pkgasset_0001","path":"assets/pkgasset_0001.png","kind":"image","mediaType":"image/png","sha256":"` + sha256Hex(fake) + `","sizeBytes":` + itoa(len(fake)) + `,"displayName":"","author":"","license":"","notice":""}]}`)
	_, template, _ := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: fake},
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrAssetTypeMismatch) {
		t.Fatalf("expected ErrAssetTypeMismatch for an executable signature, got %v", err)
	}
}

func TestReadArchive_KindMismatchRejected(t *testing.T) {
	png := validPNGBytes(t)
	manifest := []byte(`{"format":"streaming-tree-template-package","schemaVersion":1,"templatePath":"template.json","assets":[{"id":"pkgasset_0001","path":"assets/pkgasset_0001.png","kind":"video","mediaType":"image/png","sha256":"` + sha256Hex(png) + `","sizeBytes":` + itoa(len(png)) + `,"displayName":"","author":"","license":"","notice":""}]}`)
	_, template, _ := validPackageParts(t)
	data := buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: png},
	})
	if _, err := ReadArchive(data); !errors.Is(err, ErrAssetUnsupported) && !errors.Is(err, ErrAssetTypeMismatch) {
		t.Fatalf("expected a kind-mismatch error, got %v", err)
	}
}
