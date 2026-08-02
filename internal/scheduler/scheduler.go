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

// Package scheduler fires the desktop app's one daily inform run.
//
// The scheduler polls the schedule section of informer.json every tick, and runs
// the injected runner at most once per calendar day, as soon as the local clock
// reaches the configured minute. A run is therefore caught up once: an app opened
// after the configured time - or a schedule enabled that late - still pushes the
// same day, and never twice.
//
// The once-per-day guard survives restarts: the last successful run date is read
// and written through the injected persistence callbacks. A failed run does not
// consume the day, so the next tick retries.
//
// The command line entry does not use this package; a CLI run stays scheduled by
// the operator's crontab.
package scheduler

import (
	"time"

	"github.com/vogo/logger"
)

// defaultTick is how often the loop wakes to compare the clock with the schedule.
// The schedule lives in a small file read without a lock, so the poll is cheap,
// and half a minute is the most a manual change of the file waits to take effect.
const defaultTick = 30 * time.Second

// timeLayout is the "HH:MM" shape the schedule section stores.
const timeLayout = "15:04"

// dateLayout is the calendar day key the once-per-day guard records.
const dateLayout = "2006-01-02"

// runOutcome is one finished runner invocation, delivered back to the loop so only
// that goroutine mutates the once-per-day state.
type runOutcome struct {
	day string
	err error
}

// Scheduler polls a schedule and fires one runner per day. The zero value is not
// usable; build one with New, or assemble the struct in white box tests.
type Scheduler struct {
	// readConfig reports the live schedule. It is called once per tick, so a hand
	// edit of the configuration file takes effect without a restart.
	readConfig func() (enabled bool, at string, err error)

	// readLastRun returns the calendar day of the last successful run, or an empty
	// string when none is stored. It is how a restarted process learns that today
	// was already pushed.
	readLastRun func() (string, error)

	// writeLastRun records a successful run day so a later process does not catch
	// up again on the same calendar day.
	writeLastRun func(day string) error

	// runner performs one inform run and reports whether it finished cleanly.
	// It is invoked in its own goroutine, so a slow fetch never delays the loop.
	runner func() error

	// tick is the polling interval; New sets the default, tests shorten it.
	tick time.Duration

	stop    chan struct{}
	done    chan struct{}
	results chan runOutcome

	// lastRunDate is the calendar day of the last successful run known to this
	// process, empty before the first success. Only the loop goroutine touches it.
	lastRunDate string

	// inFlight is true while a runner goroutine is still outstanding. Only the
	// loop goroutine touches it, together with lastRunDate.
	inFlight bool
}

// New builds a scheduler over one schedule source, one once-per-day store and one runner.
func New(
	readConfig func() (enabled bool, at string, err error),
	readLastRun func() (string, error),
	writeLastRun func(day string) error,
	runner func() error,
) *Scheduler {
	return &Scheduler{
		readConfig:   readConfig,
		readLastRun:  readLastRun,
		writeLastRun: writeLastRun,
		runner:       runner,
		tick:         defaultTick,
	}
}

// Start launches the polling loop and returns immediately. Call it once; Stop
// ends the loop again.
func (s *Scheduler) Start() {
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	// buffer one outcome so a finishing runner never blocks on a loop that is
	// mid-check; Stop still cancels a send when the loop has already exited.
	s.results = make(chan runOutcome, 1)

	go s.loop()
}

// Stop ends the polling loop and waits for it to exit. It deliberately does not
// wait for an in-flight run: one either finishes or dies with the process, and a
// closing window must never hang behind a slow network fetch.
func (s *Scheduler) Stop() {
	close(s.stop)
	<-s.done
}

func (s *Scheduler) loop() {
	defer close(s.done)

	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			s.check(now)
		case outcome := <-s.results:
			s.handleOutcome(outcome)
		}
	}
}

// handleOutcome clears the in-flight mark and, on success only, records the day
// both in memory and through the persistence callback.
func (s *Scheduler) handleOutcome(outcome runOutcome) {
	s.inFlight = false

	if outcome.err != nil {
		logger.Warnf("scheduler: daily inform run of %s failed: %v", outcome.day, outcome.err)

		return
	}

	if s.writeLastRun != nil {
		err := s.writeLastRun(outcome.day)
		if err != nil {
			// still remember the day in this process so the next tick does not
			// deliver a second bot message after a successful push.
			logger.Warnf("scheduler: persist last run date %s failed: %v", outcome.day, err)
		}
	}

	s.lastRunDate = outcome.day
	logger.Infof("scheduler: finished the daily inform run of %s", outcome.day)
}

// check fires the runner when the schedule is due at now: enabled, the clock past
// today's configured minute, and no successful fire yet today. A broken
// configuration is logged and skipped, never fatal: a scheduler that exits on a
// typo would push nothing at all until a restart.
func (s *Scheduler) check(now time.Time) {
	if s.inFlight {
		return
	}

	enabled, at, err := s.readConfig()
	if err != nil {
		logger.Warnf("scheduler: read the schedule config failed: %v", err)

		return
	}

	if !enabled {
		return
	}

	hourMinute, err := time.Parse(timeLayout, at)
	if err != nil {
		logger.Warnf("scheduler: skip the invalid schedule time %q: %v", at, err)

		return
	}

	today := now.Format(dateLayout)
	if s.lastRunDate == today {
		return
	}

	if s.alreadyRan(today) {
		return
	}

	target := time.Date(now.Year(), now.Month(), now.Day(), hourMinute.Hour(), hourMinute.Minute(), 0, 0, now.Location())
	if now.Before(target) {
		return
	}

	s.inFlight = true
	logger.Infof("scheduler: start the daily inform run of %s", today)

	day := today

	go func() {
		runErr := s.runner()

		select {
		case s.results <- runOutcome{day: day, err: runErr}:
		case <-s.stop:
		}
	}()
}

// alreadyRan loads the persisted successful day and, when it matches today,
// caches it on lastRunDate so later ticks skip the disk read.
func (s *Scheduler) alreadyRan(today string) bool {
	if s.readLastRun == nil {
		return false
	}

	stored, err := s.readLastRun()
	if err != nil {
		logger.Warnf("scheduler: read last run date failed: %v", err)

		// refuse to fire when the store is unreadable: a duplicate bot message is
		// worse than waiting for the file to become readable again.
		return true
	}

	if stored != today {
		return false
	}

	s.lastRunDate = today

	return true
}
