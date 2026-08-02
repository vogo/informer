/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package scheduler //nolint:testpackage //white box tests drive the unexported tick body with controlled clocks, free of real timers.

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// errTestConfig simulates a broken schedule configuration source.
var errTestConfig = errors.New("the file is unreadable")

// errTestRun simulates a failed inform run.
var errTestRun = errors.New("the inform run failed")

// memoryStore is an in-memory once-per-day store for white box tests.
type memoryStore struct {
	mu     sync.Mutex
	day    string
	err    error
	writes []string
}

func (m *memoryStore) read() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.day, m.err
}

func (m *memoryStore) write(day string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.writes = append(m.writes, day)
	m.day = day

	return m.err
}

// newTestScheduler builds a scheduler over fixed answers from the schedule source.
func newTestScheduler(enabled bool, at string, configErr error, store *memoryStore, runner func() error) *Scheduler {
	if store == nil {
		store = &memoryStore{}
	}

	return &Scheduler{
		readConfig:   func() (bool, string, error) { return enabled, at, configErr },
		readLastRun:  store.read,
		writeLastRun: store.write,
		runner:       runner,
		tick:         time.Millisecond,
		results:      make(chan runOutcome, 1),
	}
}

// drainOutcome applies one buffered runner result the way the loop would.
func drainOutcome(t *testing.T, scheduler *Scheduler) {
	t.Helper()

	select {
	case outcome := <-scheduler.results:
		scheduler.handleOutcome(outcome)
	case <-time.After(time.Second):
		require.Fail(t, "the scheduler never reported a runner outcome")
	}
}

// assertFired waits for one runner invocation and drains its outcome.
func assertFired(t *testing.T, scheduler *Scheduler, fired <-chan struct{}) {
	t.Helper()

	select {
	case <-fired:
	case <-time.After(time.Second):
		require.Fail(t, "the scheduler did not fire the runner")
	}

	drainOutcome(t, scheduler)
}

// assertNotFired proves the runner stays untouched for a short, generous window.
func assertNotFired(t *testing.T, fired <-chan struct{}) {
	t.Helper()

	select {
	case <-fired:
		require.Fail(t, "the scheduler fired the runner unexpectedly")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCheckFiresOnceTheClockReachesTheTime(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 1)
	scheduler := newTestScheduler(true, "10:00", nil, nil, func() error {
		fired <- struct{}{}

		return nil
	})

	// one minute early, nothing happens.
	scheduler.check(time.Date(2026, 8, 2, 9, 59, 0, 0, time.UTC))
	assertNotFired(t, fired)

	// the exact configured minute counts as reached.
	scheduler.check(time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC))
	assertFired(t, scheduler, fired)
}

func TestCheckFiresAtMostOncePerCalendarDay(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 2)
	store := &memoryStore{}
	scheduler := newTestScheduler(true, "10:00", nil, store, func() error {
		fired <- struct{}{}

		return nil
	})

	scheduler.check(time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC))
	assertFired(t, scheduler, fired)

	// later the same day, even hours later, the run already happened.
	scheduler.check(time.Date(2026, 8, 2, 23, 59, 0, 0, time.UTC))
	assertNotFired(t, fired)

	// the next calendar day runs again.
	scheduler.check(time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC))
	assertFired(t, scheduler, fired)

	require.Equal(t, []string{"2026-08-02", "2026-08-03"}, store.writes)
}

func TestCheckCatchesUpOnceWhenOpenedAfterTheTime(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 1)
	scheduler := newTestScheduler(true, "10:00", nil, nil, func() error {
		fired <- struct{}{}

		return nil
	})

	// an app first polling at six in the evening still pushes the same day,
	// exactly once - the documented catch-up behavior.
	scheduler.check(time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC))
	assertFired(t, scheduler, fired)

	scheduler.check(time.Date(2026, 8, 2, 18, 0, 30, 0, time.UTC))
	assertNotFired(t, fired)
}

func TestCheckSkipsADayAlreadyPersistedByAPreviousProcess(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 1)
	store := &memoryStore{day: "2026-08-02"}
	scheduler := newTestScheduler(true, "10:00", nil, store, func() error {
		fired <- struct{}{}

		return nil
	})

	// a restarted app must not catch up again after a successful push earlier today.
	scheduler.check(time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC))
	assertNotFired(t, fired)
	require.Equal(t, "2026-08-02", scheduler.lastRunDate)
	require.Empty(t, store.writes)
}

func TestCheckRetriesAfterAFailedRun(t *testing.T) {
	t.Parallel()

	var attempts int
	fired := make(chan struct{}, 2)
	store := &memoryStore{}
	scheduler := newTestScheduler(true, "10:00", nil, store, func() error {
		attempts++
		fired <- struct{}{}

		if attempts == 1 {
			return errTestRun
		}

		return nil
	})

	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)

	scheduler.check(now)
	assertFired(t, scheduler, fired)
	require.Empty(t, store.writes, "a failed run must not consume the calendar day")
	require.Empty(t, scheduler.lastRunDate)

	// the next tick retries and, on success, records the day.
	scheduler.check(now)
	assertFired(t, scheduler, fired)
	require.Equal(t, []string{"2026-08-02"}, store.writes)
	require.Equal(t, "2026-08-02", scheduler.lastRunDate)
}

func TestCheckSkipsADisabledSchedule(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 1)
	scheduler := newTestScheduler(false, "10:00", nil, nil, func() error {
		fired <- struct{}{}

		return nil
	})

	scheduler.check(time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC))
	assertNotFired(t, fired)
}

func TestCheckSkipsAnInvalidTime(t *testing.T) {
	t.Parallel()

	for _, at := range []string{"", "25:00", "9:99", "ab:cd"} {
		fired := make(chan struct{}, 1)
		scheduler := newTestScheduler(true, at, nil, nil, func() error {
			fired <- struct{}{}

			return nil
		})

		scheduler.check(time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC))
		assertNotFired(t, fired)
	}
}

func TestCheckSurvivesAConfigurationError(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 1)
	scheduler := newTestScheduler(false, "", errTestConfig, nil, func() error {
		fired <- struct{}{}

		return nil
	})

	scheduler.check(time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC))
	assertNotFired(t, fired)
}

func TestStartStopFiresAndStops(t *testing.T) {
	t.Parallel()

	// midnight is already past whenever the test runs, so the first tick fires.
	fired := make(chan struct{}, 1)
	scheduler := New(
		func() (bool, string, error) { return true, "00:00", nil },
		func() (string, error) { return "", nil },
		func(string) error { return nil },
		func() error {
			fired <- struct{}{}

			return nil
		},
	)
	scheduler.tick = time.Millisecond

	scheduler.Start()

	select {
	case <-fired:
	case <-time.After(time.Second):
		require.Fail(t, "the scheduler did not fire the runner")
	}

	// Stop waits for the loop only; it must return even if the runner were slow.
	scheduler.Stop()
}
