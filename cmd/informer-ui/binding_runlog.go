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

package main

import (
	"fmt"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/vogo/informer/internal/runlog"
)

// maxRunLogEntries bounds how many lines one watched run publishes to the
// window. An agent that reads twenty pages narrates a lot, and the panel has no
// reason to hold all of it; the run keeps going, only the reporting stops.
const maxRunLogEntries = 500

// RunLogDTO is one progress line of a watched run - a test fetch or a diagnosis.
//
// It mirrors the RunLogEntry interface the pages declare: a wails event payload
// is not part of the generated bindings, so the two are kept in step by hand.
type RunLogDTO struct {
	// RunID is the token the caller passed when it started the run. A page uses
	// it to ignore the lines of a run it already moved on from.
	RunID string `json:"runId"`

	// Seq numbers the lines of one run from 1.
	Seq int `json:"seq"`

	// Time is when the line was recorded, in unix milliseconds.
	Time int64 `json:"time"`

	// Level is one of the runlog level names: info, warn or error.
	Level string `json:"level"`

	// Text is the line itself.
	Text string `json:"text"`
}

// runLogSink publishes the lines of one run to the window on one event name.
type runLogSink struct {
	event string
	runID string

	mu      sync.Mutex
	seq     int
	stopped bool
}

// Write publishes one line, and stops once the run has said enough.
func (s *runLogSink) Write(entry runlog.Entry) {
	dto, ok := s.next(entry)
	if !ok {
		return
	}

	// application.Get returns nil until a window exists, which is the case in
	// the binding tests and would be the case in any headless run; emitting has
	// to be a no-op there rather than a crash.
	app := application.Get()
	if app == nil || app.Event == nil {
		return
	}

	app.Event.Emit(s.event, dto)
}

// next numbers one entry, reporting whether it is still within the run's budget.
// The last line of an exhausted budget says so rather than going silent.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *runLogSink) next(entry runlog.Entry) (*RunLogDTO, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return nil, false
	}

	s.seq++

	if s.seq > maxRunLogEntries {
		s.stopped = true

		return &RunLogDTO{
			RunID: s.runID,
			Seq:   s.seq,
			Time:  entry.Time,
			Level: runlog.LevelWarn,
			Text:  fmt.Sprintf("日志超过 %d 条，后续省略", maxRunLogEntries),
		}, true
	}

	return &RunLogDTO{
		RunID: s.runID,
		Seq:   s.seq,
		Time:  entry.Time,
		Level: entry.Level,
		Text:  entry.Text,
	}, true
}

// runLogSinkFor returns where one watched run reports to, or nil for a caller
// that asked for no reporting. A nil sink runs exactly as an unwatched run does.
func runLogSinkFor(event, runID string) runlog.Sink {
	if runID == "" {
		return nil
	}

	return &runLogSink{event: event, runID: runID}
}
