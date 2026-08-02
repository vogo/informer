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

// Scheduler polls a schedule and fires one runner per day. The zero value is not
// usable; build one with New, or assemble the struct in white box tests.
type Scheduler struct {
	// readConfig reports the live schedule. It is called once per tick, so a hand
	// edit of the configuration file takes effect without a restart.
	readConfig func() (enabled bool, at string, err error)

	// runner performs one inform run. It is invoked in its own goroutine, so a
	// slow fetch never delays the loop, and concurrent fires from other entry
	// points are the runner's own problem to serialize.
	runner func()

	// tick is the polling interval; New sets the default, tests shorten it.
	tick time.Duration

	stop chan struct{}
	done chan struct{}

	// lastRunDate is the calendar day the scheduler last fired on, empty before
	// the first fire. Only the loop goroutine ever touches it, so it needs no lock.
	lastRunDate string
}

// New builds a scheduler over one schedule source and one runner.
func New(readConfig func() (enabled bool, at string, err error), runner func()) *Scheduler {
	return &Scheduler{
		readConfig: readConfig,
		runner:     runner,
		tick:       defaultTick,
	}
}

// Start launches the polling loop and returns immediately. Call it once; Stop
// ends the loop again.
func (s *Scheduler) Start() {
	s.stop = make(chan struct{})
	s.done = make(chan struct{})

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
		}
	}
}

// check fires the runner when the schedule is due at now: enabled, the clock past
// today's configured minute, and no fire yet today. A broken configuration is
// logged and skipped, never fatal: a scheduler that exits on a typo would push
// nothing at all until a restart.
func (s *Scheduler) check(now time.Time) {
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

	target := time.Date(now.Year(), now.Month(), now.Day(), hourMinute.Hour(), hourMinute.Minute(), 0, 0, now.Location())
	if now.Before(target) {
		return
	}

	s.lastRunDate = today
	logger.Infof("scheduler: start the daily inform run of %s", today)

	go s.runner()
}
