package remote

import (
	"context"
	"io"
	"strings"
	"testing"
)

// RunRemoteFSConformance runs a standard suite of CRUD, nesting, and
// root-listing tests against a RemoteFS implementation. Each protocol's
// test file calls this with a factory that constructs a fresh RemoteFS
// connected to a real (or mock) backend.
//
// The suite validates the RemoteFS path contract: paths are relative to
// the remote root, use forward slashes, and have no leading slash. The
// root directory is represented by "" or ".".
//
// newFS should return a fresh, connected RemoteFS. The cleanup function
// returned by newFS is registered with t.Cleanup so the caller does not
// need to manage it.
func RunRemoteFSConformance(t *testing.T, name string, newFS func(t *testing.T) (RemoteFS, func())) {
	t.Helper()

	ctx := context.Background()

	// --- ListRoot ---
	// Listing the root directory must not error. This is a regression test
	// for the SMBFS.resolve bug where the root was resolved to "/" instead
	// of ".", which go-smb2 rejects.
	t.Run(name+"/ListRoot", func(t *testing.T) {
		rfs, cleanup := newFS(t)
		t.Cleanup(cleanup)

		entries, err := rfs.List(ctx, ".")
		if err != nil {
			t.Fatalf("List '.': %v", err)
		}
		// An empty share is valid; we only check the call succeeds.
		_ = entries
	})

	// --- BasicCRUD ---
	// Write → Stat → Read → List → Remove full cycle.
	t.Run(name+"/BasicCRUD", func(t *testing.T) {
		rfs, cleanup := newFS(t)
		t.Cleanup(cleanup)

		// Write a file in a nested directory.
		if err := rfs.Write(ctx, "conformance/hello.txt", strings.NewReader("hello conformance")); err != nil {
			t.Fatalf("Write: %v", err)
		}

		// Stat it.
		info, err := rfs.Stat(ctx, "conformance/hello.txt")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if info.Size != int64(len("hello conformance")) {
			t.Errorf("Size = %d, want %d", info.Size, len("hello conformance"))
		}
		if info.IsDir {
			t.Error("expected file, not dir")
		}

		// Read it back.
		rc, err := rfs.Read(ctx, "conformance/hello.txt")
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		if string(data) != "hello conformance" {
			t.Errorf("Read = %q, want %q", string(data), "hello conformance")
		}

		// List the directory.
		entries, err := rfs.List(ctx, "conformance")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("List returned %d entries, want 1", len(entries))
		}

		// Remove it.
		if err := rfs.Remove(ctx, "conformance/hello.txt"); err != nil {
			t.Fatalf("Remove: %v", err)
		}

		// Stat should now return ErrNotExist.
		_, err = rfs.Stat(ctx, "conformance/hello.txt")
		if err == nil || !strings.Contains(err.Error(), "not exist") {
			t.Errorf("expected ErrNotExist after remove, got %v", err)
		}
	})

	// --- NestedPaths ---
	// Deeply nested directory creation via Write (mkdirParents path).
	t.Run(name+"/NestedPaths", func(t *testing.T) {
		rfs, cleanup := newFS(t)
		t.Cleanup(cleanup)

		deepPath := "a/b/c/d/e/deep.txt"
		if err := rfs.Write(ctx, deepPath, strings.NewReader("nested")); err != nil {
			t.Fatalf("Write nested: %v", err)
		}

		rc, err := rfs.Read(ctx, deepPath)
		if err != nil {
			t.Fatalf("Read nested: %v", err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		if string(data) != "nested" {
			t.Errorf("Read nested = %q, want %q", string(data), "nested")
		}
	})

	// --- OverwriteFile ---
	// Writing to an existing path must overwrite the previous content.
	t.Run(name+"/OverwriteFile", func(t *testing.T) {
		rfs, cleanup := newFS(t)
		t.Cleanup(cleanup)

		p := "overwrite-test.txt"
		if err := rfs.Write(ctx, p, strings.NewReader("first")); err != nil {
			t.Fatalf("Write first: %v", err)
		}
		if err := rfs.Write(ctx, p, strings.NewReader("second")); err != nil {
			t.Fatalf("Write second: %v", err)
		}

		rc, err := rfs.Read(ctx, p)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		if string(data) != "second" {
			t.Errorf("after overwrite, Read = %q, want %q", string(data), "second")
		}
	})

	// --- RemoveMissing ---
	// Removing a non-existent file must not error.
	t.Run(name+"/RemoveMissing", func(t *testing.T) {
		rfs, cleanup := newFS(t)
		t.Cleanup(cleanup)

		if err := rfs.Remove(ctx, "does-not-exist.txt"); err != nil {
			t.Errorf("Remove missing file should not error, got: %v", err)
		}
	})

	// --- ListEmpty ---
	// Listing an empty or non-existent directory returns an empty slice.
	t.Run(name+"/ListEmpty", func(t *testing.T) {
		rfs, cleanup := newFS(t)
		t.Cleanup(cleanup)

		entries, err := rfs.List(ctx, "empty-dir")
		if err != nil {
			t.Fatalf("List empty dir: %v", err)
		}
		if entries == nil {
			t.Error("expected non-nil empty slice, got nil")
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	// --- MkdirAll ---
	// MkdirAll creates a directory tree, and listing it works.
	t.Run(name+"/MkdirAll", func(t *testing.T) {
		rfs, cleanup := newFS(t)
		t.Cleanup(cleanup)

		if err := rfs.MkdirAll(ctx, "mktest/sub/dir"); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		info, err := rfs.Stat(ctx, "mktest/sub/dir")
		if err != nil {
			t.Fatalf("Stat after MkdirAll: %v", err)
		}
		if !info.IsDir {
			t.Error("expected directory after MkdirAll")
		}
	})
}
