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

// PreviewLogEvent is the wails event every test fetch progress line arrives on.
//
// One name carries every run: a listener tells its own lines apart by the run id
// it passed to PreviewSource. A per run event name would grow the runtime's
// listener map on every fetch, and a single missed unsubscribe would leak it.
//
// The frontend has to declare the same name; SourceManager.vue is the other half.
const PreviewLogEvent = "informer:preview:log"

// maxPreviewLogEntries bounds how many lines one test fetch publishes. An agent
// that reads twenty pages narrates a lot, and the window has no reason to hold
// all of it; the run keeps going, only the reporting stops.
const maxPreviewLogEntries = 500

// PreviewLogDTO is one progress line of a test fetch.
//
// It mirrors the PreviewLogEntry interface in SourceManager.vue: a wails event
// payload is not part of the generated bindings, so the two are kept in step by
// hand.
type PreviewLogDTO struct {
	// RunID is the token the caller passed to PreviewSource. A page uses it to
	// ignore the lines of a fetch it already moved on from.
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

// previewSink publishes the lines of one test fetch to the window.
type previewSink struct {
	runID string

	mu      sync.Mutex
	seq     int
	stopped bool
}

// Write publishes one line, and stops once the run has said enough.
func (s *previewSink) Write(entry runlog.Entry) {
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

	app.Event.Emit(PreviewLogEvent, dto)
}

// next numbers one entry, reporting whether it is still within the run's budget.
// The last line of an exhausted budget says so rather than going silent.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func (s *previewSink) next(entry runlog.Entry) (*PreviewLogDTO, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return nil, false
	}

	s.seq++

	if s.seq > maxPreviewLogEntries {
		s.stopped = true

		return &PreviewLogDTO{
			RunID: s.runID,
			Seq:   s.seq,
			Time:  entry.Time,
			Level: runlog.LevelWarn,
			Text:  fmt.Sprintf("日志超过 %d 条，后续省略", maxPreviewLogEntries),
		}, true
	}

	return &PreviewLogDTO{
		RunID: s.runID,
		Seq:   s.seq,
		Time:  entry.Time,
		Level: entry.Level,
		Text:  entry.Text,
	}, true
}

// previewSinkFor returns where one test fetch reports to, or nil for a caller
// that asked for no reporting. A nil sink parses exactly as it always did.
func previewSinkFor(runID string) runlog.Sink {
	if runID == "" {
		return nil
	}

	return &previewSink{runID: runID}
}

// PreviewSource runs one real fetch and parse of a stored subscription and
// returns the candidate articles. It writes nothing - not the source, not
// articles, not health state - and a disabled source previews just the same.
//
// runID, when not empty, streams the run's progress to the window on
// PreviewLogEvent, so a fetch that takes minutes - an agent source browses the
// web before it answers - is something the user can watch instead of wait out.
// The id is the caller's own token: the page tells its lines apart by it, which
// is what keeps two test fetches in a row from mixing.
func (a *App) PreviewSource(id int64, runID string) ([]*ArticleDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	articles, err := a.svc.PreviewTraced(id, previewSinkFor(runID))
	if err != nil {
		return nil, err
	}

	dtos := make([]*ArticleDTO, 0, len(articles))
	for _, article := range articles {
		dtos = append(dtos, &ArticleDTO{Title: article.Title, URL: article.URL})
	}

	return dtos, nil
}
