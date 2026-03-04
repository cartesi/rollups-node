// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

package pmutex

import (
	"strings"
	"sync"
	"testing"
	"time"

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
	s.mutex.LLock()
	acquired := make(chan struct{})
	go func() {
		s.mutex.HLock()
		close(acquired)
	}()
	select {
	case <-acquired:
		s.Fail("HLock should not be acquired while LLock is held")
	case <-time.After(decisecond):
		// Expected: HLock is blocked.
	}
	// Clean up: unlock so the goroutine can finish.
	s.mutex.Unlock()
	<-acquired
}

func (s *PMutexSuite) TestContestedLLock() {
	s.mutex.HLock()
	acquired := make(chan struct{})
	go func() {
		s.mutex.LLock()
		close(acquired)
	}()
	select {
	case <-acquired:
		s.Fail("LLock should not be acquired while HLock is held")
	case <-time.After(decisecond):
		// Expected: LLock is blocked.
	}
	// Clean up: unlock so the goroutine can finish.
	s.mutex.Unlock()
	<-acquired
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

const decisecond = 100 * time.Millisecond
