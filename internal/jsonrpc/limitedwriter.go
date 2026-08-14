// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"io"
)

// budgetWriter tracks the response bytes available to one HTTP request. A
// batch shares this budget across all of its entries; a single request uses the
// same budget for its sole response.
//
// Each response is first encoded into a limitedWriter and only reaches writer
// when Flush succeeds. If any response exceeds the remaining budget, that
// response is discarded atomically and the budgetWriter is permanently
// closed. NewLimitedWriter then returns nil, causing every remaining batch
// entry to receive the response-size-limit error, even if that entry's response
// would fit in the unused budget. The response that causes closure does not
// consume any budget.
type budgetWriter struct {
	writer io.Writer
	budget int
	closed bool
}

// newBudgetWriter creates a response budget of exactly limit bytes. A response
// whose encoded size equals the remaining budget is allowed.
func newBudgetWriter(writer io.Writer, limit int) *budgetWriter {
	return &budgetWriter{
		writer: writer,
		budget: limit,
	}
}

// Write commits an already-buffered response and deducts successfully written
// bytes from the shared budget.
func (w *budgetWriter) Write(data []byte) (int, error) {
	written, err := w.writer.Write(data)
	if err == nil {
		w.budget -= written
	}
	return written, err
}

// NewLimitedWriter creates an atomic buffer for the next response. It returns
// nil after any response has exceeded the shared budget.
func (w *budgetWriter) NewLimitedWriter() *limitedWriter {
	if w.closed {
		return nil
	}
	return &limitedWriter{writer: w}
}

// limitedWriter buffers one complete JSON-RPC response before committing it to
// its shared budgetWriter.
type limitedWriter struct {
	writer *budgetWriter
	buffer bytes.Buffer
}

// Write appends data while the complete buffered response fits in the remaining
// budget. An overflowing write returns io.ErrShortBuffer, discards the response
// on Flush, and permanently closes the shared budget.
func (w *limitedWriter) Write(data []byte) (int, error) {
	if w.buffer.Len()+len(data) > w.writer.budget {
		w.writer.closed = true
		return 0, io.ErrShortBuffer
	}
	return w.buffer.Write(data)
}

// Flush atomically commits the buffered response unless an overflow has closed
// the shared budget. A flush after closure is intentionally a no-op.
func (w *limitedWriter) Flush() error {
	if w.writer.closed {
		return nil
	}
	_, err := w.writer.Write(w.buffer.Bytes())
	return err
}
