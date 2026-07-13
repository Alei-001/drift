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

// maxDownloadSize caps how many bytes download() will accept from a single
// asset response. 200 MiB comfortably exceeds the largest prebuilt drift
// release archive while still bounding exposure to a misbehaving or hostile
// server that streams indefinitely.
const maxDownloadSize = 200 * 1024 * 1024 // 200 MiB

// download fetches a URL into a temporary file and returns its path. The
// caller is responsible for removing the file. When progressWriter is non-nil
// and the response includes Content-Length, a progress bar is rendered to it.
//
// The response body is wrapped in an io.LimitReader so that a server cannot
// exhaust disk by streaming past maxDownloadSize. An over-sized response is
// reported as a download error rather than silently truncated.
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

	// Allow up to maxDownloadSize+1 bytes through the limit reader: the
	// extra byte lets us distinguish an exact-fit response (legal) from
	// one that overflowed (illegal) by checking n > maxDownloadSize.
	limited := io.LimitReader(reader, maxDownloadSize+1)
	n, err := io.Copy(tmp, limited)
	if err != nil {
		os.Remove(tmp.Name())
		return "", &upgradeError{kind: "download", err: err}
	}
	if n > maxDownloadSize {
		os.Remove(tmp.Name())
		return "", &upgradeError{kind: "download", err: fmt.Errorf("download exceeds maximum size of %d bytes", maxDownloadSize)}
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

// maxExtractedBinarySize bounds how large an extracted drift binary may be.
// Real release binaries are well under 50 MiB; the 100 MiB ceiling catches
// a zip-bomb or accidental packaging of a debug build without rejecting
// legitimate large binaries.
const maxExtractedBinarySize = 100 * 1024 * 1024 // 100 MiB

// extractFromZip extracts the drift binary from a zip archive. When multiple
// entries match the binary name (drift/drift.exe), the largest is selected so
// a stray README or placeholder cannot displace the real binary. The
// extracted size is capped at maxExtractedBinarySize to reject zip bombs.
func extractFromZip(archivePath, outPath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", &upgradeError{kind: "extract", err: err}
	}
	defer r.Close()

	var match *zip.File
	var matchBase string
	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if base != "drift.exe" && base != "drift" {
			continue
		}
		// Pick the largest matching entry: a real binary is orders of
		// magnitude larger than a stray 0-byte placeholder of the same
		// name. Ties resolve to the first match (deterministic).
		if match == nil || f.UncompressedSize64 > match.UncompressedSize64 {
			match = f
			matchBase = base
		}
	}
	if match == nil {
		return "", &upgradeError{kind: "extract", err: errors.New("no drift binary in archive")}
	}
	if match.UncompressedSize64 > maxExtractedBinarySize {
		return "", &upgradeError{kind: "extract", err: fmt.Errorf("extracted binary exceeds maximum size of %d bytes", maxExtractedBinarySize)}
	}
	return matchBase, copyZipEntry(match, outPath)
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
	// Cap the copy at maxExtractedBinarySize+1 so an entry whose declared
	// UncompressedSize64 was correct (small) but whose actual stream is
	// huge cannot exhaust disk. The +1 lets us detect overflow exactly.
	n, err := io.Copy(dst, io.LimitReader(src, maxExtractedBinarySize+1))
	if err != nil {
		return err
	}
	if n > maxExtractedBinarySize {
		return fmt.Errorf("extracted entry exceeds maximum size of %d bytes", maxExtractedBinarySize)
	}
	return nil
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
