// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package linewriter

import (
	"bytes"
	"io"
)

// LineWriter accumulates the received data in a buffer and writes it to the inner writer when it
// encounters a new line, ignoring empty lines in the process.
// LineWriter assumes the inner writer does not returns an error.
type LineWriter struct {
	inner  io.Writer
	buffer bytes.Buffer
}

func New(inner io.Writer) *LineWriter {
	return &LineWriter{
		inner: inner,
	}
}

func (w *LineWriter) Write(data []byte) (int, error) {
	_, err := w.buffer.Write(data)
	if err != nil {
		// Not possible given bytes.Buffer spec
		panic(err)
	}
	for {
		if !bytes.ContainsRune(w.buffer.Bytes(), '\n') {
			break
		}
		line, err := w.buffer.ReadBytes('\n')
		if err != nil {
			// Not possible because we looked for the \n rune
			panic(err)
		}
		if len(line) > 1 {
			if _, err := w.inner.Write(line); err != nil {
				// Assume the writer doesn't return an error
				panic(err)
			}
		}
	}
	return len(data), nil
}
