package mediamtx

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxExtractedBytes bounds the total size written during extraction.
//
// The v1.19.3 archives are around 30 MB unpacked; 256 MB leaves generous room
// while still stopping a decompression bomb from filling the disk.
const maxExtractedBytes = 256 << 20

// maxArchiveEntries bounds how many members an archive may contain. The
// official archives hold three files.
const maxArchiveEntries = 256

// extractedFile records one file written during extraction.
type extractedFile struct {
	// Name is the archive-relative name, always a bare file name here.
	Name string
	// Path is the absolute path it was written to.
	Path string
	Mode os.FileMode
}

// extractArchive unpacks an archive into destDir, refusing anything unsafe.
//
// Rejected outright:
//   - absolute entry paths,
//   - entries containing "..",
//   - entries that resolve outside destDir,
//   - symlinks and hard links of any kind,
//   - device, socket and other irregular entries,
//   - archives over the entry or byte budget.
//
// Links are refused rather than validated. The official archives contain none,
// so accepting them would only add an attack surface: a link resolved after
// extraction can point anywhere, and validating that safely is far harder than
// declining it.
func extractArchive(archivePath, destDir string, format ArchiveFormat) ([]extractedFile, error) {
	switch format {
	case FormatZip:
		return extractZip(archivePath, destDir)
	case FormatTarGz:
		return extractTarGz(archivePath, destDir)
	default:
		return nil, fmt.Errorf("%w: unknown archive format %q", ErrArchiveInvalid, format)
	}
}

// safeEntryPath validates an archive entry name and returns its target path.
func safeEntryPath(destDir, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: archive contains an entry with an empty name", ErrArchiveInvalid)
	}

	// Normalise separators: zip always uses "/", tar should, but a hostile
	// archive may use "\" to sneak past a naive check on Windows.
	normalized := strings.ReplaceAll(name, "\\", "/")

	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("%w: archive entry %q is an absolute path", ErrArchiveInvalid, name)
	}
	// A Windows drive letter such as "C:/..." is absolute too.
	if len(normalized) >= 2 && normalized[1] == ':' {
		return "", fmt.Errorf("%w: archive entry %q is an absolute path", ErrArchiveInvalid, name)
	}

	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return "", fmt.Errorf(
				"%w: archive entry %q escapes the extraction directory", ErrArchiveInvalid, name)
		}
	}

	target := filepath.Join(destDir, filepath.FromSlash(normalized))

	// Belt and braces: confirm the joined path really is inside destDir, which
	// also catches anything the segment check missed.
	relative, err := filepath.Rel(destDir, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf(
			"%w: archive entry %q resolves outside the extraction directory", ErrArchiveInvalid, name)
	}

	return target, nil
}

// copyBounded copies at most remaining bytes, reporting the amount written.
func copyBounded(dst io.Writer, src io.Reader, remaining int64) (int64, error) {
	// Read one byte past the budget so exceeding it is detectable.
	written, err := io.Copy(dst, io.LimitReader(src, remaining+1))
	if err != nil {
		return written, err
	}
	if written > remaining {
		return written, fmt.Errorf(
			"%w: archive contents exceed the %d byte extraction limit",
			ErrArchiveInvalid, maxExtractedBytes)
	}
	return written, nil
}

func extractZip(archivePath, destDir string) ([]extractedFile, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read the zip archive: %v", ErrArchiveInvalid, err)
	}
	defer reader.Close()

	if len(reader.File) > maxArchiveEntries {
		return nil, fmt.Errorf("%w: archive has %d entries, more than the %d allowed",
			ErrArchiveInvalid, len(reader.File), maxArchiveEntries)
	}

	var (
		files     []extractedFile
		remaining = int64(maxExtractedBytes)
	)

	for _, entry := range reader.File {
		info := entry.FileInfo()

		if info.IsDir() {
			// Directories are created implicitly for the files kept below; the
			// official archive is flat, so nothing needs an explicit mkdir.
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf(
				"%w: archive entry %q is a symlink", ErrArchiveInvalid, entry.Name)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf(
				"%w: archive entry %q is not a regular file", ErrArchiveInvalid, entry.Name)
		}

		target, err := safeEntryPath(destDir, entry.Name)
		if err != nil {
			return nil, err
		}

		written, err := writeZipEntry(entry, target, info.Mode(), remaining)
		if err != nil {
			return nil, err
		}
		remaining -= written

		files = append(files, extractedFile{
			Name: filepath.Base(target),
			Path: target,
			Mode: info.Mode(),
		})
	}

	return files, nil
}

func writeZipEntry(entry *zip.File, target string, mode os.FileMode, remaining int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return 0, fmt.Errorf("create extraction directory: %w", err)
	}

	source, err := entry.Open()
	if err != nil {
		return 0, fmt.Errorf("%w: cannot read archive entry %q: %v", ErrArchiveInvalid, entry.Name, err)
	}
	defer source.Close()

	// O_EXCL: an archive containing the same name twice must fail loudly rather
	// than let the second entry overwrite the first.
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return 0, fmt.Errorf("create extracted file: %w", err)
	}
	defer file.Close()

	written, err := copyBounded(file, source, remaining)
	if err != nil {
		return written, err
	}

	return written, file.Close()
}

func extractTarGz(archivePath, destDir string) ([]extractedFile, error) {
	handle, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer handle.Close()

	gzipReader, err := gzip.NewReader(handle)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read the gzip archive: %v", ErrArchiveInvalid, err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	var (
		files     []extractedFile
		remaining = int64(maxExtractedBytes)
		entries   int
	)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: cannot read the tar archive: %v", ErrArchiveInvalid, err)
		}

		entries++
		if entries > maxArchiveEntries {
			return nil, fmt.Errorf("%w: archive has more than the %d entries allowed",
				ErrArchiveInvalid, maxArchiveEntries)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeSymlink, tar.TypeLink:
			return nil, fmt.Errorf(
				"%w: archive entry %q is a link", ErrArchiveInvalid, header.Name)
		case tar.TypeReg:
			// Handled below.
		default:
			return nil, fmt.Errorf(
				"%w: archive entry %q is not a regular file", ErrArchiveInvalid, header.Name)
		}

		target, err := safeEntryPath(destDir, header.Name)
		if err != nil {
			return nil, err
		}

		mode := header.FileInfo().Mode()
		written, err := writeTarEntry(tarReader, target, mode, remaining)
		if err != nil {
			return nil, err
		}
		remaining -= written

		files = append(files, extractedFile{
			Name: filepath.Base(target),
			Path: target,
			Mode: mode,
		})
	}

	return files, nil
}

func writeTarEntry(reader io.Reader, target string, mode os.FileMode, remaining int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return 0, fmt.Errorf("create extraction directory: %w", err)
	}

	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return 0, fmt.Errorf("create extracted file: %w", err)
	}
	defer file.Close()

	written, err := copyBounded(file, reader, remaining)
	if err != nil {
		return written, err
	}

	return written, file.Close()
}

// findExtracted returns the file with the given base name.
func findExtracted(files []extractedFile, name string) (extractedFile, bool) {
	for _, file := range files {
		if file.Name == name {
			return file, true
		}
	}
	return extractedFile{}, false
}
