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
