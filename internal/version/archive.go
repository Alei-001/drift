package version

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/schollz/progressbar/v3"
)

// download fetches a URL into a temporary file and returns its path. The
// caller is responsible for removing the file. When progressWriter is non-nil
// and the response includes Content-Length, a progress bar is rendered to it.
func download(ctx context.Context, url string, progressWriter io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", &upgradeError{kind: "download", err: err}
	}
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", &upgradeError{kind: "download", err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &upgradeError{kind: "download", err: fmt.Errorf("asset download returned %s", resp.Status)}
	}

	tmp, err := os.CreateTemp("", "drift-upgrade-*")
	if err != nil {
		return "", &upgradeError{kind: "download", err: err}
	}
	defer tmp.Close()

	reader := io.Reader(resp.Body)
	if progressWriter != nil && resp.ContentLength > 0 {
		bar := progressbar.NewOptions64(
			resp.ContentLength,
			progressbar.OptionSetWriter(progressWriter),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetDescription("downloading"),
			progressbar.OptionSetWidth(20),
		)
		r := progressbar.NewReader(resp.Body, bar)
		reader = &r
		defer bar.Finish()
	}

	if _, err := io.Copy(tmp, reader); err != nil {
		os.Remove(tmp.Name())
		return "", &upgradeError{kind: "download", err: err}
	}
	return tmp.Name(), nil
}

// sha256File returns the hex-encoded SHA-256 of the file at path.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyChecksum parses a checksums file (SHA256 lines, sha256sum format)
// and returns true when the expected hash for the named asset matches the
// actual hash of the downloaded asset file.
func verifyChecksum(checksumsPath, assetName, assetPath string) (bool, error) {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return false, &upgradeError{kind: "checksum", err: err}
	}
	want, err := lookupChecksum(string(data), assetName)
	if err != nil {
		return false, &upgradeError{kind: "checksum", err: err}
	}
	got, err := sha256File(assetPath)
	if err != nil {
		return false, &upgradeError{kind: "checksum", err: err}
	}
	return got == want, nil
}

// lookupChecksum scans sha256sum-format content for the entry matching name.
// Format per line: "<hex>  <name>" (two spaces, per GNU coreutils).
func lookupChecksum(content, name string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %q", name)
}

// extractBinary extracts the drift executable from a release archive.
// The archive is a .zip (windows) or .tar.gz (other) containing a single
// binary named "drift" or "drift.exe". The extracted binary is written to
// outPath. Returns the actual binary name found inside the archive.
func extractBinary(archivePath, outPath, goos string) (string, error) {
	if goos == "windows" {
		return extractFromZip(archivePath, outPath)
	}
	return extractFromTarGz(archivePath, outPath)
}

// extractFromZip extracts the drift binary from a zip archive.
func extractFromZip(archivePath, outPath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", &upgradeError{kind: "extract", err: err}
	}
	defer r.Close()

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if base == "drift.exe" || base == "drift" {
			return base, copyZipEntry(f, outPath)
		}
	}
	return "", &upgradeError{kind: "extract", err: errors.New("no drift binary in archive")}
}

func copyZipEntry(f *zip.File, outPath string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

// extractFromTarGz extracts the drift binary from a .tar.gz archive.
func extractFromTarGz(archivePath, outPath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", &upgradeError{kind: "extract", err: err}
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", &upgradeError{kind: "extract", err: err}
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", &upgradeError{kind: "extract", err: err}
		}
		base := filepath.Base(hdr.Name)
		if base == "drift" || base == "drift.exe" {
			return base, writeTarEntry(tr, outPath, hdr.FileInfo())
		}
	}
	return "", &upgradeError{kind: "extract", err: errors.New("no drift binary in archive")}
}

func writeTarEntry(r io.Reader, outPath string, fi os.FileInfo) error {
	dst, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode())
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, r)
	return err
}
