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

package logbuf_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/logbuf"
	"github.com/vogo/informer/internal/runlog"
)

// stamped renders one line the way the global logger writes it, so the parsing
// under test is exercised on the real prefix rather than on a guess about it.
func stamped(tag, text string) string {
	return "2026/08/19 21:25:33.123 " + tag + " " + text + "\n"
}

func TestWriteParsesLevelAndStripsThePrefix(t *testing.T) {
	t.Parallel()

	buffer := logbuf.New(10)

	for _, line := range []string{
		stamped("INFO", "fetch a.com"),
		stamped("WARN", "slow: 3s"),
		stamped("ERRO", "broken: no match"),
		stamped("DEBG", "candidate dropped"),
	} {
		_, err := buffer.Write([]byte(line))
		require.NoError(t, err)
	}

	snapshot := buffer.Since(0, 0)

	require.Len(t, snapshot.Entries, 4)
	require.Equal(t, int64(4), snapshot.LatestSeq)
	require.Zero(t, snapshot.Dropped)
	require.Equal(t, 10, snapshot.Capacity)

	require.Equal(t, logbuf.LevelInfo, snapshot.Entries[0].Level)
	require.Equal(t, "fetch a.com", snapshot.Entries[0].Text)
	require.Equal(t, int64(1), snapshot.Entries[0].Seq)
	require.Positive(t, snapshot.Entries[0].Time)

	require.Equal(t, logbuf.LevelWarn, snapshot.Entries[1].Level)
	require.Equal(t, "slow: 3s", snapshot.Entries[1].Text)

	require.Equal(t, logbuf.LevelError, snapshot.Entries[2].Level)
	require.Equal(t, "broken: no match", snapshot.Entries[2].Text)

	require.Equal(t, logbuf.LevelDebug, snapshot.Entries[3].Level)
}

func TestRecordKeepsAnUnprefixedLineWhole(t *testing.T) {
	t.Parallel()

	buffer := logbuf.New(10)

	buffer.Record("gorm: query articles by source")
	buffer.Record("2026/08/19 21:25:33.123 HUH? something else entirely")
	buffer.Record("   \n  ")

	snapshot := buffer.Since(0, 0)

	require.Len(t, snapshot.Entries, 2, "a blank record records nothing")
	require.Equal(t, "gorm: query articles by source", snapshot.Entries[0].Text)
	require.Equal(t, logbuf.LevelInfo, snapshot.Entries[0].Level)
	require.Equal(t, "2026/08/19 21:25:33.123 HUH? something else entirely", snapshot.Entries[1].Text)
}

func TestContinuationLinesInheritTheLevelOfTheirRecord(t *testing.T) {
	t.Parallel()

	buffer := logbuf.New(10)

	buffer.Record(stamped("ERRO", "agent failed:\n  at step 1\n  at step 2"))

	snapshot := buffer.Since(0, 0)

	require.Len(t, snapshot.Entries, 3)

	for _, entry := range snapshot.Entries {
		require.Equal(t, logbuf.LevelError, entry.Level)
	}

	require.Equal(t, "agent failed:", snapshot.Entries[0].Text)
	require.Equal(t, "at step 1", snapshot.Entries[1].Text)
}

//nolint:gosmopolitan //the truncation notice is chinese, like the rest of the product.
func TestOneOversizedLineIsCut(t *testing.T) {
	t.Parallel()

	buffer := logbuf.New(10)

	buffer.Record(stamped("INFO", strings.Repeat("x", logbuf.MaxLineRunes+500)))

	entries := buffer.Since(0, 0).Entries

	require.Len(t, entries, 1)
	require.Contains(t, entries[0].Text, "已截断")
	require.Less(t, len([]rune(entries[0].Text)), logbuf.MaxLineRunes+100)
}

func TestTheRingKeepsTheNewestLinesAndReportsWhatItLost(t *testing.T) {
	t.Parallel()

	buffer := logbuf.New(3)

	for i := 1; i <= 5; i++ {
		buffer.Record(stamped("INFO", fmt.Sprintf("line %d", i)))
	}

	require.Equal(t, 3, buffer.Len())

	snapshot := buffer.Since(0, 0)

	require.Len(t, snapshot.Entries, 3)
	require.Equal(t, "line 3", snapshot.Entries[0].Text)
	require.Equal(t, "line 5", snapshot.Entries[2].Text)
	require.Equal(t, int64(2), snapshot.Dropped, "the two overwritten lines are reported, not hidden")
	require.Equal(t, int64(5), snapshot.LatestSeq)
}

func TestSinceReturnsOnlyWhatTheCursorHasNotSeen(t *testing.T) {
	t.Parallel()

	buffer := logbuf.New(10)

	buffer.Record(stamped("INFO", "one"))
	buffer.Record(stamped("INFO", "two"))

	first := buffer.Since(0, 0)
	require.Len(t, first.Entries, 2)

	buffer.Record(stamped("INFO", "three"))

	second := buffer.Since(first.LatestSeq, 0)

	require.Len(t, second.Entries, 1)
	require.Equal(t, "three", second.Entries[0].Text)
	require.Zero(t, second.Dropped)

	// polling again with nothing new answers empty rather than repeating itself.
	require.Empty(t, buffer.Since(second.LatestSeq, 0).Entries)
}

func TestALimitKeepsTheNewestLines(t *testing.T) {
	t.Parallel()

	buffer := logbuf.New(10)

	for i := 1; i <= 6; i++ {
		buffer.Record(stamped("INFO", fmt.Sprintf("line %d", i)))
	}

	snapshot := buffer.Since(0, 2)

	require.Len(t, snapshot.Entries, 2)
	require.Equal(t, "line 5", snapshot.Entries[0].Text)
	require.Equal(t, "line 6", snapshot.Entries[1].Text)
	require.Equal(t, int64(4), snapshot.Dropped)
	require.Equal(t, int64(6), snapshot.LatestSeq, "the cursor moves past what the limit held back")
}

func TestAStaleCursorAheadOfTheBufferAnswersEmpty(t *testing.T) {
	t.Parallel()

	buffer := logbuf.New(10)

	buffer.Record(stamped("INFO", "one"))

	snapshot := buffer.Since(99, 0)

	require.Empty(t, snapshot.Entries)
	require.Zero(t, snapshot.Dropped)
	require.Equal(t, int64(1), snapshot.LatestSeq)
}

func TestClearForgetsTheLinesButKeepsSequencesMovingForward(t *testing.T) {
	t.Parallel()

	buffer := logbuf.New(10)

	buffer.Record(stamped("INFO", "before"))
	cursor := buffer.Since(0, 0).LatestSeq

	buffer.Clear()

	require.Zero(t, buffer.Len())
	require.Empty(t, buffer.Since(cursor, 0).Entries)

	buffer.Record(stamped("INFO", "after"))

	snapshot := buffer.Since(cursor, 0)

	require.Len(t, snapshot.Entries, 1)
	require.Equal(t, "after", snapshot.Entries[0].Text)
	require.Equal(t, int64(2), snapshot.Entries[0].Seq, "a reused sequence would resurrect a cleared line")
}

func TestAZeroCapacityFallsBackToTheDefault(t *testing.T) {
	t.Parallel()

	require.Equal(t, logbuf.DefaultCapacity, logbuf.New(0).Capacity())
	require.Equal(t, logbuf.DefaultCapacity, logbuf.New(-1).Capacity())
}

func TestConcurrentWritersDoNotLoseOrRepeatASequence(t *testing.T) {
	t.Parallel()

	const writers, each = 8, 50

	buffer := logbuf.New(writers * each)

	var group sync.WaitGroup

	for w := range writers {
		group.Go(func() {
			for i := range each {
				buffer.Record(stamped("INFO", fmt.Sprintf("w%d-%d", w, i)))
			}
		})
	}

	group.Wait()

	entries := buffer.Since(0, 0).Entries

	require.Len(t, entries, writers*each)

	for i, entry := range entries {
		require.Equal(t, int64(i+1), entry.Seq)
	}
}

// TestInstallTeesTheGlobalLoggerWithoutStealingIt is the one case that touches
// the process wide logger, so it stays sequential.
//
//nolint:paralleltest //it swaps the global logger output.
func TestInstallTeesTheGlobalLoggerWithoutStealingIt(t *testing.T) {
	original := logger.Writer()

	t.Cleanup(func() { logger.SetOutput(original) })

	console := &strings.Builder{}
	logger.SetOutput(console)

	require.Same(t, logbuf.Default(), logbuf.Install())
	require.Same(t, logbuf.Default(), logbuf.Install(), "installing twice must not stack a second tee")

	cursor := logbuf.Default().Since(0, 0).LatestSeq

	runlog.Warnf(nil, "captured %s", "once")

	entries := logbuf.Default().Since(cursor, 0).Entries

	require.Len(t, entries, 1, "a second tee would record the line twice")
	require.Equal(t, logbuf.LevelWarn, entries[0].Level)
	require.Equal(t, "captured once", entries[0].Text)
	require.Contains(t, console.String(), "captured once", "stdout keeps getting every line")
}
