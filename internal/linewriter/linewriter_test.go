// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package linewriter

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type mockWriter struct {
	mock.Mock
}

func (w *mockWriter) Write(p []byte) (int, error) {
	args := w.Called(p)
	return args.Int(0), args.Error(1)
}

type LineWriterSuite struct {
	suite.Suite
	mock   *mockWriter
	writer *LineWriter
}

func TestLineWriterSuite(t *testing.T) {
	suite.Run(t, &LineWriterSuite{})
}

func (s *LineWriterSuite) SetupTest() {
	s.mock = &mockWriter{}
	s.writer = New(s.mock)
}

func (s *LineWriterSuite) TestItWritesLines() {
	s.mock.On("Write", mock.Anything).Return(0, nil)
	lines := [][]byte{
		[]byte("hello\n"),
		[]byte("world\n"),
		[]byte("nugget\n"),
	}
	for _, line := range lines {
		n, err := s.writer.Write(line)
		s.Equal(n, len(line))
		s.Nil(err)
	}
	for _, line := range lines {
		s.mock.AssertCalled(s.T(), "Write", line)
	}
}

func (s *LineWriterSuite) TestItSplitMultipleLines() {
	s.mock.On("Write", mock.Anything).Return(0, nil)
	lines := [][]byte{
		[]byte("hello\n"),
		[]byte("world\n"),
		[]byte("nugget\n"),
	}
	data := bytes.Join(lines, nil)
	n, err := s.writer.Write(data)
	s.Equal(n, len(data))
	s.Nil(err)
	for _, line := range lines {
		s.mock.AssertCalled(s.T(), "Write", line)
	}
}

func (s *LineWriterSuite) TestItIgnoresEmptyLines() {
	s.mock.On("Write", mock.Anything).Return(0, nil)
	lines := [][]byte{
		[]byte("hello\n"),
		[]byte("world\n"),
		[]byte("nugget\n"),
	}
	parts := [][]byte{
		{'\n'},
		{'\n'},
		lines[0],
		{'\n'},
		{'\n'},
		lines[1],
		{'\n'},
		{'\n'},
		lines[2],
		{'\n'},
		{'\n'},
	}
	for _, part := range parts {
		n, err := s.writer.Write(part)
		s.Equal(n, len(part))
		s.Nil(err)
	}
	for _, line := range lines {
		s.mock.AssertCalled(s.T(), "Write", line)
	}
}

func (s *LineWriterSuite) TestItDoesNotWriteStringWithoutNewLine() {
	s.mock.On("Write", mock.Anything).Return(0, nil)
	data := []byte("hello nugget")
	n, err := s.writer.Write(data)
	s.Equal(n, len(data))
	s.Nil(err)
	s.mock.AssertNotCalled(s.T(), "Write")
}

func (s *LineWriterSuite) TestItJoinsStringsIntoSingleLine() {
	s.mock.On("Write", mock.Anything).Return(0, nil)
	parts := [][]byte{
		[]byte("hello "),
		[]byte("world "),
		[]byte("nugget"),
		[]byte("\n"),
	}
	line := bytes.Join(parts, nil)
	for _, part := range parts {
		n, err := s.writer.Write(part)
		s.Equal(n, len(part))
		s.Nil(err)
	}
	s.mock.AssertCalled(s.T(), "Write", line)
}

func (s *LineWriterSuite) TestItPanicsIfWriterReturnsError() {
	s.mock.On("Write", mock.Anything).Return(0, errors.New("test error"))
	s.PanicsWithError("test error", func() {
		_, _ = s.writer.Write([]byte("hello nugget\n"))
	})
}
