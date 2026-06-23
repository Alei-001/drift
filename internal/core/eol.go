package core

import (
	"bytes"
	"io"
)

type crlfToLFWriter struct {
	w io.Writer
}

func NewLFWriter(w io.Writer) io.Writer {
	return &crlfToLFWriter{w: w}
}

func (c *crlfToLFWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	var n int
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

	if data[len(data)-1] == '\r' {
		rest, err := c.w.Write(data[n : len(data)-1])
		if err != nil {
			return n + rest, err
		}
		return n + rest + 1, nil
	}

	rest, err := c.w.Write(data[n:])
	return n + rest, err
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
