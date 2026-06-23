package core

import (
	"bytes"
	"io"
)

type crlfToLFWriter struct {
	w         io.Writer
	pendingCR bool
}

func NewLFWriter(w io.Writer) *crlfToLFWriter {
	return &crlfToLFWriter{w: w}
}

func (c *crlfToLFWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	var n int

	// Merge pending \r from previous call with leading \n.
	if c.pendingCR && data[0] == '\n' {
		if _, err := c.w.Write([]byte{'\n'}); err != nil {
			return 0, err
		}
		c.pendingCR = false
		n++ // \n consumed from this call's input; keep data unsliced
	} else if c.pendingCR {
		// Lone \r from previous call — write it now.
		if _, err := c.w.Write([]byte{'\r'}); err != nil {
			return 0, err
		}
		c.pendingCR = false
	}

	if len(data) == 0 {
		return n, nil
	}

	for {
		window := data[n:]

		idx := bytes.Index(window, []byte{'\r', '\n'})
		if idx == -1 {
			break
		}

		w, err := c.w.Write(window[:idx])
		if err != nil {
			return n + w, err
		}
		_, err = c.w.Write([]byte{'\n'})
		if err != nil {
			return n + w, err
		}

		n += idx + 2
	}

	// Buffer trailing \r — it may be the start of \r\n in the next Write call.
	if len(data[n:]) > 0 && data[len(data)-1] == '\r' {
		rest, err := c.w.Write(data[n : len(data)-1])
		if err != nil {
			return n + rest, err
		}
		c.pendingCR = true
		return n + rest + 1, nil
	}

	rest, err := c.w.Write(data[n:])
	return n + rest, err
}

// Close flushes any buffered \r. Call after the last Write to ensure a
// trailing lone \r is not lost.
func (c *crlfToLFWriter) Close() error {
	if c.pendingCR {
		_, err := c.w.Write([]byte{'\r'})
		c.pendingCR = false
		return err
	}
	return nil
}

type lfToCRLFWriter struct {
	w     io.Writer
	hadCR bool
}

func NewCRLFWriter(w io.Writer) io.Writer {
	return &lfToCRLFWriter{w: w}
}

func (c *lfToCRLFWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	var n int
	for {
		window := data[n:]

		idx := bytes.IndexByte(window, '\n')
		if idx == -1 {
			break
		}

		switch {
		case idx == 0 && c.hadCR:
			fallthrough
		case idx > 0 && window[idx-1] == '\r':
			w, err := c.w.Write(window[:idx+1])
			if err != nil {
				return n + w, err
			}
		default:
			w, err := c.w.Write(window[:idx])
			if err != nil {
				return n + w, err
			}
			_, err = c.w.Write([]byte{'\r', '\n'})
			if err != nil {
				return n + w, err
			}
		}

		n += idx + 1
	}

	c.hadCR = data[len(data)-1] == '\r'

	rest, err := c.w.Write(data[n:])
	return n + rest, err
}
