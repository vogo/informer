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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// errTestConfig simulates a broken schedule configuration source.
var errTestConfig = errors.New("the file is unreadable")

// newTestScheduler builds a scheduler over fixed answers from the schedule source.
func newTestScheduler(enabled bool, at string, configErr error, runner func()) *Scheduler {
	return &Scheduler{
		readConfig: func() (bool, string, error) { return enabled, at, configErr },
		runner:     runner,
		tick:       time.Millisecond,
	}
}

// assertFired waits for one runner invocation.
func assertFired(t *testing.T, fired <-chan struct{}) {
	t.Helper()

	select {
	case <-fired:
	case <-time.After(time.Second):
		require.Fail(t, "the scheduler did not fire the runner")
	}
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
	scheduler := newTestScheduler(true, "10:00", nil, func() { fired <- struct{}{} })

	// one minute early, nothing happens.
	scheduler.check(time.Date(2026, 8, 2, 9, 59, 0, 0, time.UTC))
	assertNotFired(t, fired)

	// the exact configured minute counts as reached.
	scheduler.check(time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC))
	assertFired(t, fired)
}

func TestCheckFiresAtMostOncePerCalendarDay(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 2)
	scheduler := newTestScheduler(true, "10:00", nil, func() { fired <- struct{}{} })

	scheduler.check(time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC))
	assertFired(t, fired)

	// later the same day, even hours later, the run already happened.
	scheduler.check(time.Date(2026, 8, 2, 23, 59, 0, 0, time.UTC))
	assertNotFired(t, fired)

	// the next calendar day runs again.
	scheduler.check(time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC))
	assertFired(t, fired)
}

func TestCheckCatchesUpOnceWhenOpenedAfterTheTime(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 1)
	scheduler := newTestScheduler(true, "10:00", nil, func() { fired <- struct{}{} })

	// an app first polling at six in the evening still pushes the same day,
	// exactly once - the documented catch-up behavior.
	scheduler.check(time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC))
	assertFired(t, fired)

	scheduler.check(time.Date(2026, 8, 2, 18, 0, 30, 0, time.UTC))
	assertNotFired(t, fired)
}

func TestCheckSkipsADisabledSchedule(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 1)
	scheduler := newTestScheduler(false, "10:00", nil, func() { fired <- struct{}{} })

	scheduler.check(time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC))
	assertNotFired(t, fired)
}

func TestCheckSkipsAnInvalidTime(t *testing.T) {
	t.Parallel()

	for _, at := range []string{"", "25:00", "9:99", "ab:cd"} {
		fired := make(chan struct{}, 1)
		scheduler := newTestScheduler(true, at, nil, func() { fired <- struct{}{} })

		scheduler.check(time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC))
		assertNotFired(t, fired)
	}
}

func TestCheckSurvivesAConfigurationError(t *testing.T) {
	t.Parallel()

	fired := make(chan struct{}, 1)
	scheduler := newTestScheduler(false, "", errTestConfig, func() { fired <- struct{}{} })

	scheduler.check(time.Date(2026, 8, 2, 18, 0, 0, 0, time.UTC))
	assertNotFired(t, fired)
}

func TestStartStopFiresAndStops(t *testing.T) {
	t.Parallel()

	// midnight is already past whenever the test runs, so the first tick fires.
	fired := make(chan struct{}, 1)
	scheduler := newTestScheduler(true, "00:00", nil, func() { fired <- struct{}{} })

	scheduler.Start()
	assertFired(t, fired)

	// Stop waits for the loop only; it must return even if the runner were slow.
	scheduler.Stop()
}
