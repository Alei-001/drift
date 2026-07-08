package version

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestMatchAsset(t *testing.T) {
	assets := []Asset{
		{Name: "drift_v1.0.0_windows_amd64.zip", BrowserDownloadURL: "u1"},
		{Name: "drift_v1.0.0_linux_amd64.tar.gz", BrowserDownloadURL: "u2"},
		{Name: "drift_v1.0.0_darwin_arm64.tar.gz", BrowserDownloadURL: "u3"},
		{Name: "drift_v1.0.0_checksums.txt", BrowserDownloadURL: "u4"},
	}

	cases := []struct {
		os, arch string
		want     string
		wantErr  bool
	}{
		{"windows", "amd64", "drift_v1.0.0_windows_amd64.zip", false},
		{"linux", "amd64", "drift_v1.0.0_linux_amd64.tar.gz", false},
		{"darwin", "arm64", "drift_v1.0.0_darwin_arm64.tar.gz", false},
		{"freebsd", "amd64", "", true}, // unsupported platform
		{"linux", "arm64", "", true},   // no asset for this combo
	}
	for _, c := range cases {
		got, err := matchAsset(assets, c.os, c.arch)
		if c.wantErr {
			if err == nil {
				t.Errorf("matchAsset(%s,%s): expected error, got %s", c.os, c.arch, got.Name)
			}
			continue
		}
		if err != nil {
			t.Errorf("matchAsset(%s,%s): unexpected error %v", c.os, c.arch, err)
			continue
		}
		if got.Name != c.want {
			t.Errorf("matchAsset(%s,%s): got %s, want %s", c.os, c.arch, got.Name, c.want)
		}
	}
}

func TestFindChecksumAsset(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		assets := []Asset{
			{Name: "drift_v1.0.0_linux_amd64.tar.gz"},
			{Name: "drift_v1.0.0_checksums.txt"},
		}
		_, ok := findChecksumAsset(assets)
		if !ok {
			t.Error("expected checksum asset to be found")
		}
	})
	t.Run("absent", func(t *testing.T) {
		assets := []Asset{
			{Name: "drift_v1.0.0_linux_amd64.tar.gz"},
		}
		_, ok := findChecksumAsset(assets)
		if ok {
			t.Error("expected no checksum asset")
		}
	})
}

func TestLookupChecksum(t *testing.T) {
	content := `abc123  drift_v1.0.0_linux_amd64.tar.gz
def456  drift_v1.0.0_windows_amd64.zip
`
	got, err := lookupChecksum(content, "drift_v1.0.0_windows_amd64.zip")
	if err != nil {
		t.Fatal(err)
	}
	if got != "def456" {
		t.Errorf("got %s, want def456", got)
	}
	if _, err := lookupChecksum(content, "missing.tar.gz"); err == nil {
		t.Error("expected error for missing entry")
	}
}

func TestVerifyChecksum(t *testing.T) {
	// Build a real archive + matching checksums file.
	payload := []byte("this is the drift binary payload")
	archivePath := makeTempFile(t, payload)
	defer os.Remove(archivePath)

	want := sha256.Sum256(payload)
	wantHex := hex.EncodeToString(want[:])
	assetName := "drift_v1.0.0_linux_amd64.tar.gz"
	checksums := wantHex + "  " + assetName + "\n"
	csPath := makeTempFile(t, []byte(checksums))
	defer os.Remove(csPath)

	t.Run("match", func(t *testing.T) {
		ok, err := verifyChecksum(csPath, assetName, archivePath)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Error("expected checksum to match")
		}
	})
	t.Run("mismatch", func(t *testing.T) {
		badArchive := makeTempFile(t, []byte("different content"))
		defer os.Remove(badArchive)
		ok, err := verifyChecksum(csPath, assetName, badArchive)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Error("expected checksum mismatch")
		}
	})
}

func TestExtractBinary_Zip(t *testing.T) {
	// Build a zip containing drift.exe.
	payload := []byte("windows drift binary")
	zipPath := buildZip(t, "drift.exe", payload)
	defer os.Remove(zipPath)

	outPath := filepath.Join(t.TempDir(), "drift.exe")
	name, err := extractFromZip(zipPath, outPath)
	if err != nil {
		t.Fatal(err)
	}
	if name != "drift.exe" {
		t.Errorf("got name %s, want drift.exe", name)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("extracted content mismatch: got %q, want %q", got, payload)
	}
}

func TestExtractBinary_TarGz(t *testing.T) {
	payload := []byte("unix drift binary")
	tarPath := buildTarGz(t, "drift", payload)
	defer os.Remove(tarPath)

	outPath := filepath.Join(t.TempDir(), "drift")
	name, err := extractFromTarGz(tarPath, outPath)
	if err != nil {
		t.Fatal(err)
	}
	if name != "drift" {
		t.Errorf("got name %s, want drift", name)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("extracted content mismatch: got %q, want %q", got, payload)
	}
}

func TestExtractBinary_NoBinary(t *testing.T) {
	// Zip with an unrelated file.
	zipPath := buildZip(t, "README.txt", []byte("not a binary"))
	defer os.Remove(zipPath)
	outPath := filepath.Join(t.TempDir(), "drift.exe")
	_, err := extractFromZip(zipPath, outPath)
	if err == nil {
		t.Error("expected error when archive has no drift binary")
	}
}

// buildZip creates a zip archive at a temp path containing a single file
// named name with the given content. Returns the archive path.
func buildZip(t *testing.T, name string, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "test-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	fw, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

// buildTarGz creates a .tar.gz archive at a temp path containing a single
// file named name with the given content.
func buildTarGz(t *testing.T, name string, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "test-*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	return f.Name()
}

// makeTempFile writes content to a new temp file and returns its path.
func makeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "drift-test-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
