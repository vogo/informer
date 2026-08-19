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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/runlog"
)

// firstLine is the text of the first recorded line every budget case starts from.
const firstLine = "one"

// TestPreviewSourceWithARunIDNeedsNoWindow is the regression test of the nil
// application guard: every binding test runs in a process that never called
// application.New, and a preview asking for progress must not crash there.
func TestPreviewSourceWithARunIDNeedsNoWindow(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(sampleAtom))
	}))
	defer server.Close()

	app := newTestApp(t)

	req := sampleRequest()
	req.URL = server.URL

	created, err := app.CreateSource(req)
	require.NoError(t, err)

	articles, err := app.PreviewSource(created.ID, "run-1")
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.Equal(t, "Hello Preview", articles[0].Title)
}

func TestPreviewSinkForSkipsAnEmptyRunID(t *testing.T) {
	t.Parallel()

	assert.Nil(t, previewSinkFor(""))
	assert.NotNil(t, previewSinkFor("run-1"))
}

//nolint:gosmopolitan //asserting on the shipped chinese notice.
func TestRunLogSinkNumbersLinesAndStopsAtTheBudget(t *testing.T) {
	t.Parallel()

	sink := &runLogSink{event: PreviewLogEvent, runID: "run-1"}

	first, ok := sink.next(runlog.Entry{Time: 7, Level: runlog.LevelInfo, Text: firstLine})
	require.True(t, ok)
	assert.Equal(t, "run-1", first.RunID)
	assert.Equal(t, 1, first.Seq)
	assert.Equal(t, int64(7), first.Time)
	assert.Equal(t, firstLine, first.Text)

	for range maxRunLogEntries - 1 {
		_, ok = sink.next(runlog.Entry{Level: runlog.LevelInfo, Text: "filler"})
		require.True(t, ok)
	}

	// the line that exhausts the budget says so instead of going silent.
	last, ok := sink.next(runlog.Entry{Level: runlog.LevelInfo, Text: "dropped"})
	require.True(t, ok)
	assert.Equal(t, runlog.LevelWarn, last.Level)
	assert.Contains(t, last.Text, "后续省略")

	_, ok = sink.next(runlog.Entry{Level: runlog.LevelInfo, Text: "also dropped"})
	assert.False(t, ok)
}

// TestPreviewSinkWriteNeedsNoWindow covers the emit path itself, not just the
// numbering: application.Get is nil here and Write has to notice.
func TestPreviewSinkWriteNeedsNoWindow(t *testing.T) {
	t.Parallel()

	sink := previewSinkFor("run-1")

	require.NotPanics(t, func() {
		runlog.Infof(sink, "a line")
	})
}
