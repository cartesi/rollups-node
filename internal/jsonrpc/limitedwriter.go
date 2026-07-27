// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"io"
)

type budgetWriter struct {
	writer io.Writer
	budget int
	closed bool
}

func newBudgetWriter(writer io.Writer, limit int) *budgetWriter {
	return &budgetWriter{
		writer: writer,
		budget: limit,
	}
}

func (w *budgetWriter) Write(data []byte) (int, error) {
	written, err := w.writer.Write(data)
	if err == nil {
		w.budget -= written
	}
	return written, err
}

func (w *budgetWriter) NewLimitedWriter() *limitedWriter {
	if w.closed {
		return nil
	}
	return &limitedWriter{writer: w}
}

type limitedWriter struct {
	writer *budgetWriter
	buffer bytes.Buffer
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	if w.buffer.Len()+len(data) > w.writer.budget {
		w.writer.closed = true
		return 0, io.ErrShortBuffer
	}
	return w.buffer.Write(data)
}

func (w *limitedWriter) Flush() error {
	if w.writer.closed {
		return nil
	}
	_, err := w.writer.Write(w.buffer.Bytes())
	return err
}
