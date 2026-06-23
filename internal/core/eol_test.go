package core

import (
	"bytes"
	"io"
	"testing"
)

func TestCRLFToLF(t *testing.T) {
	input := []byte("a\r\nb\r\nc")
	expected := []byte("a\nb\nc")

	var buf bytes.Buffer
	w := NewLFWriter(&buf)
	n, err := w.Write(input)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(input) {
		t.Errorf("n = %d, want %d", n, len(input))
	}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("output = %q, want %q", buf.Bytes(), expected)
	}
}

func TestCRLFToLF_NoCRLF(t *testing.T) {
	input := []byte("plain text\nno crlf here")
	var buf bytes.Buffer
	w := NewLFWriter(&buf)
	w.Write(input)
	if !bytes.Equal(buf.Bytes(), input) {
		t.Errorf("output = %q, want %q", buf.Bytes(), input)
	}
}

func TestCRLFToLF_TrailingCR(t *testing.T) {
	writes := [][]byte{
		[]byte("hel"),
		[]byte("lo\r"),
		[]byte("\nworld"),
	}
	expected := []byte("hello\nworld")

	var buf bytes.Buffer
	w := NewLFWriter(&buf)
	for _, d := range writes {
		w.Write(d)
	}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("output = %q, want %q", buf.Bytes(), expected)
	}
}

func TestCRLFToLF_LoneTrailingCR(t *testing.T) {
	// A lone \r at the end (not part of \r\n) should be preserved.
	// Close() flushes the buffered \r.
	input := []byte("hello\r")
	expected := []byte("hello\r")

	var buf bytes.Buffer
	w := NewLFWriter(&buf)
	n, err := w.Write(input)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(input) {
		t.Errorf("n = %d, want %d", n, len(input))
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("output = %q, want %q", buf.Bytes(), expected)
	}
}

func TestCRLFToLF_CRNotFollowedByLF(t *testing.T) {
	// \r should only be removed when immediately followed by \n.
	input := []byte("a\rb\nc\r\n")
	expected := []byte("a\rb\nc\n")

	var buf bytes.Buffer
	w := NewLFWriter(&buf)
	w.Write(input)
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("output = %q, want %q", buf.Bytes(), expected)
	}
}

func TestLFToCRLF(t *testing.T) {
	input := []byte("a\nb\nc")
	expected := []byte("a\r\nb\r\nc")

	var buf bytes.Buffer
	w := NewCRLFWriter(&buf)
	n, err := w.Write(input)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(input) {
		t.Errorf("n = %d, want %d", n, len(input))
	}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("output = %q, want %q", buf.Bytes(), expected)
	}
}

func TestLFToCRLF_ExistingCRLF(t *testing.T) {
	input := []byte("a\r\nb\r\nc\r\n")
	expected := []byte("a\r\nb\r\nc\r\n")

	var buf bytes.Buffer
	w := NewCRLFWriter(&buf)
	w.Write(input)
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("output = %q, want %q", buf.Bytes(), expected)
	}
}

func TestLFToCRLF_Mixed(t *testing.T) {
	input := []byte("a\nb\r\nc\nd")
	expected := []byte("a\r\nb\r\nc\r\nd")

	var buf bytes.Buffer
	w := NewCRLFWriter(&buf)
	w.Write(input)
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("output = %q, want %q", buf.Bytes(), expected)
	}
}

func TestLFToCRLF_HadCRState(t *testing.T) {
	writes := [][]byte{
		[]byte("hel"),
		[]byte("lo\r"),
		[]byte("\ngood"),
		[]byte("bye\n"),
	}
	expected := []byte("hello\r\ngoodbye\r\n")

	var buf bytes.Buffer
	w := NewCRLFWriter(&buf)
	for _, d := range writes {
		w.Write(d)
	}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("output = %q, want %q", buf.Bytes(), expected)
	}
}

func TestRoundTrip_CRLF_LF_CRLF(t *testing.T) {
	original := []byte("line1\r\nline2\r\nline3\r\n")

	var lf bytes.Buffer
	NewLFWriter(&lf).Write(original)

	var crlf bytes.Buffer
	NewCRLFWriter(&crlf).Write(lf.Bytes())

	if !bytes.Equal(crlf.Bytes(), original) {
		t.Errorf("round-trip failed: %q != %q", crlf.Bytes(), original)
	}
}

// TestCRLFToLF_ByteCountAcrossWrites verifies that the return values of
// Write calls across \r\n splitting correctly sum to the total input length.
// Regression test for the pendingCR + empty-data byte-count edge case.
func TestCRLFToLF_ByteCountAcrossWrites(t *testing.T) {
	var buf bytes.Buffer
	w := NewLFWriter(&buf)

	// Scenario: \r lands at end of one Write, \n starts the next.
	n1, err := w.Write([]byte("hello\r"))
	if err != nil {
		t.Fatalf("first Write failed: %v", err)
	}
	n2, err := w.Write([]byte("\nworld"))
	if err != nil {
		t.Fatalf("second Write failed: %v", err)
	}

	totalBytes := len("hello\r") + len("\nworld")
	if n1+n2 != totalBytes {
		t.Errorf("total returned bytes = %d, want %d", n1+n2, totalBytes)
	}
	if string(buf.Bytes()) != "hello\nworld" {
		t.Errorf("output = %q, want %q", buf.Bytes(), "hello\nworld")
	}
}

// TestCRLFToLF_ByteCount_JustNewline verifies the exact edge case where
// the second Write is a single \n (leaving data empty after slice).
func TestCRLFToLF_ByteCount_JustNewline(t *testing.T) {
	var buf bytes.Buffer
	w := NewLFWriter(&buf)

	n1, err := w.Write([]byte("\r"))
	if err != nil {
		t.Fatalf("first Write failed: %v", err)
	}
	n2, err := w.Write([]byte("\n"))
	if err != nil {
		t.Fatalf("second Write failed: %v", err)
	}

	totalBytes := len("\r") + len("\n")
	if n1+n2 != totalBytes {
		t.Errorf("total returned bytes = %d, want %d", n1+n2, totalBytes)
	}
	// Close to flush the pending state (should be a no-op here since \r+\n
	// combined into a single \n).
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if string(buf.Bytes()) != "\n" {
		t.Errorf("output = %q, want %q", buf.Bytes(), "\n")
	}
}

func TestEmptyWrite(t *testing.T) {
	for _, w := range []io.Writer{NewLFWriter(io.Discard), NewCRLFWriter(io.Discard)} {
		n, err := w.Write([]byte{})
		if err != nil {
			t.Errorf("Write([]byte{}) returned error: %v", err)
		}
		if n != 0 {
			t.Errorf("Write([]byte{}) returned n=%d, want 0", n)
		}
	}
}
