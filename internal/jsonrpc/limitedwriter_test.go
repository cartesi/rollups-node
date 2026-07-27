// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package jsonrpc

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitedWriterFlushesWithinBudget(t *testing.T) {
	var output bytes.Buffer
	budget := newBudgetWriter(&output, 5)
	writer := budget.NewLimitedWriter()
	require.NotNil(t, writer)

	written, err := writer.Write([]byte("he"))
	require.NoError(t, err)
	assert.Equal(t, 2, written)

	written, err = writer.Write([]byte("llo"))
	require.NoError(t, err)
	assert.Equal(t, 3, written)
	assert.Empty(t, output.String(), "writes should remain buffered until Flush")
	assert.Equal(t, 5, budget.budget, "buffered writes should not consume the budget")

	require.NoError(t, writer.Flush())
	assert.Equal(t, "hello", output.String())
	assert.Zero(t, budget.budget)
}

func TestLimitedWriterRejectsWriteBeyondBudget(t *testing.T) {
	var output bytes.Buffer
	budget := newBudgetWriter(&output, 4)
	writer := budget.NewLimitedWriter()
	require.NotNil(t, writer)

	written, err := writer.Write([]byte("abc"))
	require.NoError(t, err)
	assert.Equal(t, 3, written)

	written, err = writer.Write([]byte("de"))
	assert.ErrorIs(t, err, io.ErrShortBuffer)
	assert.Zero(t, written)
	assert.Nil(t, budget.NewLimitedWriter(), "exceeding the budget should close the budget writer")

	require.NoError(t, writer.Flush())
	assert.Empty(t, output.String(), "a response that exceeded the budget should be discarded")
	assert.Equal(t, 4, budget.budget)
}

func TestLimitedWritersShareBudget(t *testing.T) {
	var output bytes.Buffer
	budget := newBudgetWriter(&output, 6)

	first := budget.NewLimitedWriter()
	require.NotNil(t, first)
	_, err := first.Write([]byte("one"))
	require.NoError(t, err)
	require.NoError(t, first.Flush())

	second := budget.NewLimitedWriter()
	require.NotNil(t, second)
	_, err = second.Write([]byte("two"))
	require.NoError(t, err)
	require.NoError(t, second.Flush())

	assert.Equal(t, "onetwo", output.String())
	assert.Zero(t, budget.budget)

	third := budget.NewLimitedWriter()
	require.NotNil(t, third)
	written, err := third.Write([]byte("x"))
	assert.ErrorIs(t, err, io.ErrShortBuffer)
	assert.Zero(t, written)
	assert.Nil(t, budget.NewLimitedWriter())
}

func TestLimitedWriterAllowsEmptyWriteAtExhaustedBudget(t *testing.T) {
	budget := newBudgetWriter(io.Discard, 0)
	writer := budget.NewLimitedWriter()
	require.NotNil(t, writer)

	written, err := writer.Write(nil)
	require.NoError(t, err)
	assert.Zero(t, written)
	assert.False(t, budget.closed)
	require.NoError(t, writer.Flush())
}

func TestLimitedWriterFlushPropagatesWriterError(t *testing.T) {
	expectedErr := errors.New("write failed")
	underlying := &stubWriter{written: 2, err: expectedErr}
	budget := newBudgetWriter(underlying, 4)
	writer := budget.NewLimitedWriter()
	require.NotNil(t, writer)

	_, err := writer.Write([]byte("data"))
	require.NoError(t, err)

	err = writer.Flush()
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []byte("data"), underlying.data)
	assert.Equal(t, 4, budget.budget, "a failed underlying write should not consume budget")
}

type stubWriter struct {
	written int
	err     error
	data    []byte
}

func (w *stubWriter) Write(data []byte) (int, error) {
	w.data = append(w.data, data...)
	return w.written, w.err
}
