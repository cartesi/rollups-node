// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package pmutex

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestPMutex(t *testing.T) {
	suite.Run(t, new(PMutexSuite))
}

type PMutexSuite struct {
	suite.Suite
	mutex *PMutex
}

func (s *PMutexSuite) SetupTest() {
	require := s.Require()
	s.mutex = New()
	require.NotNil(s.mutex)
}

func (s *PMutexSuite) TestNew() {
	// This test is inside SetupTest.
}

func (s *PMutexSuite) TestSingleHLock() {
	s.mutex.HLock()
}

func (s *PMutexSuite) TestSingleLLock() {
	s.mutex.LLock()
}

func (s *PMutexSuite) TestContestedHLock() {
	require := s.Require()
	s.mutex.LLock()
	never(require, func() bool { s.mutex.HLock(); return true })
}

func (s *PMutexSuite) TestContestedLLock() {
	require := s.Require()
	s.mutex.HLock()
	never(require, func() bool { s.mutex.LLock(); return true })
}

func (s *PMutexSuite) TestPriority() {
	require := s.Require()
	release := make(chan struct{})

	low, high := 500, 5
	actual, expected := "", strings.Repeat("H", high)+strings.Repeat("L", low)
	var wg sync.WaitGroup
	wg.Add(low + high)

	// Creates a thread that holds the lock until we signalize its release.
	go func() {
		s.mutex.HLock()
		<-release
		s.mutex.Unlock()
	}()

	// Gives some time for the thread to hang on <-release.
	time.Sleep(decisecond)

	// Creates a lot of low-priority threads.
	for i := 0; i < low; i++ {
		go func() {
			s.mutex.LLock()
			actual += "L"
			s.mutex.Unlock()
			wg.Done()
		}()
	}

	// Creates a few high-priority threads.
	for i := 0; i < high; i++ {
		go func() {
			s.mutex.HLock()
			actual += "H"
			s.mutex.Unlock()
			wg.Done()
		}()
	}

	// Gives some time for the new threads to hang on their calls to LLock and HLock.
	time.Sleep(decisecond)

	// Releases the lock from the first thread and waits for all threads to finish.
	release <- struct{}{}
	wg.Wait()

	// Asserts that all the high-priority threads acquired the lock
	// before any of the low-priority threads.
	require.Equal(expected, actual)
}

// ------------------------------------------------------------------------------------------------

const (
	centisecond = 10 * time.Millisecond
	decisecond  = 100 * time.Millisecond
)

func never(require *require.Assertions, f func() bool) {
	require.Never(f, decisecond, centisecond)
}
