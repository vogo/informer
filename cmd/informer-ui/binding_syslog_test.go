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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/logbuf"
)

// recordSystemLine feeds one line into the process wide buffer the bindings read,
// rendered with the prefix the global logger writes.
func recordSystemLine(tag, text string) {
	logbuf.Default().Record("2026/08/19 21:25:33.123 " + tag + " " + text)
}

// TestSystemLogsReadsTheProcessBufferIncrementally and its siblings share the
// one process wide buffer, so they must not run next to each other.
//
//nolint:paralleltest //the default log buffer is process wide state.
func TestSystemLogsReadsTheProcessBufferIncrementally(t *testing.T) {
	app := newTestApp(t)

	start := app.SystemLogs(0, 0).LatestSeq

	recordSystemLine("INFO", "inform run started")
	recordSystemLine("ERRO", "source 3 failed")

	page := app.SystemLogs(start, 0)

	require.Len(t, page.Entries, 2)
	assert.Equal(t, "info", page.Entries[0].Level)
	assert.Equal(t, "inform run started", page.Entries[0].Text)
	assert.Equal(t, "error", page.Entries[1].Level)
	assert.Positive(t, page.Entries[0].Time)
	assert.Equal(t, logbuf.DefaultCapacity, page.Capacity)
	assert.Zero(t, page.Dropped)

	// polling on the returned cursor answers empty rather than repeating lines.
	assert.Empty(t, app.SystemLogs(page.LatestSeq, 0).Entries)
}

//nolint:paralleltest //the default log buffer is process wide state.
func TestSystemLogsKeepsTheNewestLinesWhenTheLimitBites(t *testing.T) {
	app := newTestApp(t)

	start := app.SystemLogs(0, 0).LatestSeq

	recordSystemLine("INFO", "older")
	recordSystemLine("INFO", "newer")

	page := app.SystemLogs(start, 1)

	require.Len(t, page.Entries, 1)
	assert.Equal(t, "newer", page.Entries[0].Text)
	assert.Equal(t, int64(1), page.Dropped)
	assert.Equal(t, start+2, page.LatestSeq, "the cursor moves past what the limit held back")
}

//nolint:paralleltest //the default log buffer is process wide state.
func TestSystemLogsAnswersOnAnAppThatFailedToStart(t *testing.T) {
	// a data directory that is a file, not a directory, is the cheapest way to
	// get an app carrying a startup error; reading the log is exactly what the
	// user does then, so it must not be behind the ready guard.
	broken := &App{initErr: assert.AnError}

	require.Error(t, broken.ready())

	start := broken.SystemLogs(0, 0).LatestSeq

	recordSystemLine("ERRO", "startup failed: disk is read only")

	page := broken.SystemLogs(start, 0)

	require.Len(t, page.Entries, 1)
	assert.Equal(t, "startup failed: disk is read only", page.Entries[0].Text)
}

//nolint:paralleltest //the default log buffer is process wide state.
func TestClearSystemLogsReturnsTheCursorToResumeFrom(t *testing.T) {
	app := newTestApp(t)

	recordSystemLine("INFO", "before the clear")

	cursor := app.ClearSystemLogs()

	assert.Zero(t, logbuf.Default().Len())
	assert.Empty(t, app.SystemLogs(cursor, 0).Entries)
	assert.Zero(t, app.SystemLogs(cursor, 0).Dropped, "the page must not be told it lost what it cleared itself")

	recordSystemLine("INFO", "after the clear")

	page := app.SystemLogs(cursor, 0)

	require.Len(t, page.Entries, 1)
	assert.Equal(t, "after the clear", page.Entries[0].Text)
}

//nolint:paralleltest //the default log buffer is process wide state.
func TestSystemLogsCapsAnOversizedLimit(t *testing.T) {
	app := newTestApp(t)

	// the batch cap is what keeps one poll from serializing an unbounded slice;
	// asking for more than the buffer can ever hold is simply clamped.
	page := app.SystemLogs(0, maxSystemLogBatch*10)

	assert.LessOrEqual(t, len(page.Entries), maxSystemLogBatch)
}
