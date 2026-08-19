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

import "github.com/vogo/informer/internal/logbuf"

// maxSystemLogBatch bounds one SystemLogs answer. The window polls while its log
// panel is open, so a batch only has to cover what a second of a busy inform run
// produces; the first load asks for the whole buffer and is bounded by it.
const maxSystemLogBatch = 2000

// SystemLogEntryDTO is one line of the process log. It mirrors logbuf.Entry:
// the frontend shape is declared here so the desktop contract never leaks an
// internal type, exactly like every other DTO of this binding layer.
type SystemLogEntryDTO struct {
	// Seq numbers the lines of this process run from 1 and never repeats. The
	// page passes the highest one it saw back as the cursor of its next poll.
	Seq int64 `json:"seq"`

	// Time is when the line was recorded, in unix milliseconds.
	Time int64 `json:"time"`

	// Level is one of the logbuf level names: debug, info, warn or error.
	Level string `json:"level"`

	// Text is the line without its timestamp and level prefix.
	Text string `json:"text"`
}

// SystemLogPageDTO is one incremental read of the process log.
type SystemLogPageDTO struct {
	// Entries are the lines after the requested cursor, oldest first.
	Entries []*SystemLogEntryDTO `json:"entries"`

	// Dropped counts the lines after the cursor that are gone for good, because
	// the buffer overwrote them. The page says so rather than presenting a log
	// with a silent hole in it.
	Dropped int64 `json:"dropped"`

	// LatestSeq is the cursor for the next poll; it moves even when nothing was
	// returned.
	LatestSeq int64 `json:"latestSeq"`

	// Capacity is how many lines the process keeps at most, shown by the panel
	// so the bound is stated rather than guessed.
	Capacity int `json:"capacity"`
}

// SystemLogs returns the recorded log lines after afterSeq, newest kept first
// dropped when there are more than limit of them. A zero afterSeq loads the
// whole buffer; a limit of zero or less falls back to maxSystemLogBatch.
//
// It deliberately skips the ready guard every data touching binding runs: a
// startup that failed is precisely when the log is worth reading, and this call
// reaches no database - only the in memory buffer of this process.
func (a *App) SystemLogs(afterSeq int64, limit int) *SystemLogPageDTO {
	if limit <= 0 || limit > maxSystemLogBatch {
		limit = maxSystemLogBatch
	}

	snapshot := logbuf.Default().Since(afterSeq, limit)

	entries := make([]*SystemLogEntryDTO, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entries = append(entries, &SystemLogEntryDTO{
			Seq:   entry.Seq,
			Time:  entry.Time,
			Level: entry.Level,
			Text:  entry.Text,
		})
	}

	return &SystemLogPageDTO{
		Entries:   entries,
		Dropped:   snapshot.Dropped,
		LatestSeq: snapshot.LatestSeq,
		Capacity:  snapshot.Capacity,
	}
}

// ClearSystemLogs forgets every recorded line and returns the cursor to poll on
// from now, so the page that cleared the view is not told afterwards that it
// dropped the lines it threw away itself.
func (a *App) ClearSystemLogs() int64 {
	buffer := logbuf.Default()

	buffer.Clear()

	return buffer.Since(0, 0).LatestSeq
}
