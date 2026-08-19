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

//nolint:testpackage //the stream reader is one command line's output format, not an api.
package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// notes collects what an observer was told, the shape every case here asserts on.
type notes struct {
	levels []string
	texts  []string
}

func (n *notes) Note(level, text string) {
	n.levels = append(n.levels, level)
	n.texts = append(n.texts, text)
}

// joined renders the collected notes as one searchable document.
func (n *notes) joined() string {
	return strings.Join(n.texts, "\n")
}

// streamFixture is one realistic run: the session starts, the agent says what it
// is doing, searches, reads a page and answers.
//
//nolint:gosmopolitan //the narration this asserts on is chinese by design.
const streamFixture = `{"type":"system","subtype":"init","tools":["WebSearch","WebFetch"]}
{"type":"assistant","message":{"content":[{"type":"text","text":"我先搜索一下。"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"WebSearch","input":{"query":"go 1.24 release"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"a result body"}]}}
{"type":"stream_event","event":{"type":"content_block_delta"}}
not json at all
{"type":"result","subtype":"success","is_error":false,"duration_ms":1200,"num_turns":3,"result":"{\"items\":[]}"}`

//nolint:gosmopolitan //the narration this asserts on is chinese by design.
func TestReadClaudeStreamNarratesTheRunAndReturnsTheAnswer(t *testing.T) {
	t.Parallel()

	watcher := &notes{}

	answer, err := readClaudeStream(strings.NewReader(streamFixture), watcher)

	require.NoError(t, err)
	require.JSONEq(t, `{"items":[]}`, answer)

	document := watcher.joined()
	require.Contains(t, document, "会话已启动")
	require.Contains(t, document, "WebSearch, WebFetch")
	require.Contains(t, document, "我先搜索一下。")
	require.Contains(t, document, `调用 WebSearch {"query":"go 1.24 release"}`)
	require.Contains(t, document, "工具返回")

	// the unknown event type and the line that is not json at all are skipped,
	// not narrated and not fatal: a newer command line must not break a run.
	require.NotContains(t, document, "not json at all")
	require.NotContains(t, document, "content_block_delta")
}

func TestReadClaudeStreamWorksWithoutAnObserver(t *testing.T) {
	t.Parallel()

	answer, err := readClaudeStream(strings.NewReader(streamFixture), nil)

	require.NoError(t, err)
	require.JSONEq(t, `{"items":[]}`, answer)
}

//nolint:gosmopolitan //asserting on the shipped chinese narration.
func TestReadClaudeStreamSurvivesAHugeLine(t *testing.T) {
	t.Parallel()

	// one fetched page handed back as a tool result easily passes the 64KB line
	// limit of a bufio.Scanner, which is why this package does not use one.
	huge := strings.Repeat("x", 512*1024)
	stream := `{"type":"user","message":{"content":[{"type":"tool_result","content":"` + huge + `"}]}}` + "\n" +
		`{"type":"result","subtype":"success","is_error":false,"result":"done"}`

	watcher := &notes{}

	answer, err := readClaudeStream(strings.NewReader(stream), watcher)

	require.NoError(t, err)
	require.Equal(t, "done", answer)
	require.Contains(t, watcher.joined(), "工具返回")
}

func TestReadClaudeStreamReportsAnErrorResult(t *testing.T) {
	t.Parallel()

	stream := `{"type":"result","subtype":"error_max_turns","is_error":true,"result":"too many turns"}`

	_, err := readClaudeStream(strings.NewReader(stream), nil)

	require.ErrorIs(t, err, ErrAgentFailed)
	require.Contains(t, err.Error(), "error_max_turns")
	require.Contains(t, err.Error(), "too many turns")
}

func TestReadClaudeStreamReportsAStreamWithoutAResult(t *testing.T) {
	t.Parallel()

	stream := "command not found: claude\nsome other noise\n"

	_, err := readClaudeStream(strings.NewReader(stream), nil)

	require.ErrorIs(t, err, ErrAgentFailed)
	require.ErrorIs(t, err, errNoResultEvent)
	// the tail of what was printed is quoted, so the reason is not lost.
	require.Contains(t, err.Error(), "command not found: claude")
}

func TestKeepTailKeepsOnlyTheLastLines(t *testing.T) {
	t.Parallel()

	var tail []string

	for _, line := range []string{"1", "2", "3", "4", "5", "6", "7"} {
		tail = keepTail(tail, line)
	}

	require.Equal(t, []string{"3", "4", "5", "6", "7"}, tail)
}

func TestCompactJSONRendersAToolInputOnOneLine(t *testing.T) {
	t.Parallel()

	require.JSONEq(t, `{"url":"https://a.com"}`, compactJSON([]byte("{\n  \"url\": \"https://a.com\"\n}")))
	require.Empty(t, compactJSON(nil))
	require.Equal(t, "not json", compactJSON([]byte("not json")))
}
