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

package runlog_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/runlog"
)

// collector is a Sink that keeps what it was given, the shape every caller of
// this package needs in its own tests.
type collector struct {
	mu      sync.Mutex
	entries []runlog.Entry
}

func (c *collector) Write(entry runlog.Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = append(c.entries, entry)
}

func TestHelpersRecordLevelAndRenderedText(t *testing.T) {
	t.Parallel()

	sink := &collector{}

	runlog.Infof(sink, "fetch %s", "a.com")
	runlog.Warnf(sink, "slow: %ds", 3)
	runlog.Errorf(sink, "broken: %v", "no match")

	require.Len(t, sink.entries, 3)

	require.Equal(t, runlog.LevelInfo, sink.entries[0].Level)
	require.Equal(t, "fetch a.com", sink.entries[0].Text)
	require.Positive(t, sink.entries[0].Time)

	require.Equal(t, runlog.LevelWarn, sink.entries[1].Level)
	require.Equal(t, "slow: 3s", sink.entries[1].Text)

	require.Equal(t, runlog.LevelError, sink.entries[2].Level)
	require.Equal(t, "broken: no match", sink.entries[2].Text)
}

func TestHelpersAreSafeWithoutASink(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		runlog.Infof(nil, "no sink")
		runlog.Warnf(nil, "no sink")
		runlog.Errorf(nil, "no sink")
		runlog.Log(nil, runlog.LevelInfo, "no sink")

		var empty runlog.FuncSink

		empty.Write(runlog.Entry{})
	})
}

func TestLogNormalizesAnUnknownLevel(t *testing.T) {
	t.Parallel()

	sink := &collector{}

	runlog.Log(sink, "shout", "hello")
	runlog.Log(sink, runlog.LevelError, "boom")

	require.Len(t, sink.entries, 2)
	require.Equal(t, runlog.LevelInfo, sink.entries[0].Level)
	require.Equal(t, runlog.LevelError, sink.entries[1].Level)
}

func TestFuncSinkAdaptsAFunction(t *testing.T) {
	t.Parallel()

	var got []string

	sink := runlog.FuncSink(func(entry runlog.Entry) {
		got = append(got, entry.Text)
	})

	runlog.Infof(sink, "one")
	runlog.Infof(sink, "two")

	require.Equal(t, []string{"one", "two"}, got)
}

//nolint:gosmopolitan //the point of the case is that chinese is cut on rune boundaries.
func TestTruncateCutsOnRuneBoundaries(t *testing.T) {
	t.Parallel()

	require.Equal(t, "短文本", runlog.Truncate("  短文本  ", 10))
	require.Equal(t, "无限制的中文", runlog.Truncate("无限制的中文", 0))

	cut := runlog.Truncate(strings.Repeat("中", 12), 5)
	require.True(t, strings.HasPrefix(cut, strings.Repeat("中", 5)))
	require.Contains(t, cut, "共 12 字符")
	require.True(t, strings.HasPrefix(cut, "中中中中中..."))
}
