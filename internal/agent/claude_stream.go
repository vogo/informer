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

package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Event types of the stream-json output informer reacts to. Everything else -
// stream deltas, usage records, session bookkeeping - is skipped rather than
// refused, so a command line that adds an event type does not break a run.
const (
	eventSystem    = "system"
	eventAssistant = "assistant"
	eventUser      = "user"
	eventResult    = "result"
)

// Content block types inside an assistant or tool result message.
const (
	blockText       = "text"
	blockToolUse    = "tool_use"
	blockToolResult = "tool_result"
)

// claudeEvent is one line of the stream. Only the fields informer acts on are
// declared; cost, token usage and session id are deliberately ignored.
type claudeEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	IsError bool   `json:"is_error"`

	// Result and Error carry the outcome of the closing result event.
	Result string `json:"result"`
	Error  string `json:"error"`

	// DurationMS and NumTurns describe the finished run.
	DurationMS int64 `json:"duration_ms"`
	NumTurns   int   `json:"num_turns"`

	// Tools is the tool set the session announced at startup.
	Tools []string `json:"tools"`

	// Message carries the content blocks of an assistant or user event.
	Message *claudeMessage `json:"message"`
}

// claudeMessage is the anthropic message shape wrapped in a stream event.
type claudeMessage struct {
	Content []claudeBlock `json:"content"`
}

// claudeBlock is one content block of a message.
type claudeBlock struct {
	Type string `json:"type"`

	// Text is the block's prose, for a text block.
	Text string `json:"text"`

	// Name and Input describe a tool call.
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`

	// Content is what a tool answered. It is either a string or a list of
	// blocks depending on the tool, so it stays raw until it is narrated.
	Content json.RawMessage `json:"content"`
}

// decodeClaudeEvent reads one stream line, reporting whether it was json this
// package understands. A line that is not - a stray warning printed on stdout,
// an event shape from a newer command line - is skipped, never fatal.
func decodeClaudeEvent(line string) (*claudeEvent, bool) {
	var event claudeEvent

	err := json.Unmarshal([]byte(line), &event)
	if err != nil || event.Type == "" {
		return nil, false
	}

	return &event, true
}

// claudeResultText unwraps the closing result event into the answer text.
func claudeResultText(event *claudeEvent) (string, error) {
	if event.IsError {
		reason := event.Error
		if reason == "" {
			reason = event.Result
		}

		return "", fmt.Errorf("%w: claude reported %q: %s", ErrAgentFailed, event.Subtype, truncate(reason))
	}

	return event.Result, nil
}

// narrate turns one stream event into the plain language an observer shows.
func narrate(event *claudeEvent, observer Observer) {
	if observer == nil {
		return
	}

	switch event.Type {
	case eventSystem:
		narrateSystem(event, observer)
	case eventAssistant, eventUser:
		narrateBlocks(event, observer)
	}
}

// narrateSystem reports the session start, the one system event worth a line.
//
//nolint:gosmopolitan //informer is a chinese product; the notes speak the user's language.
func narrateSystem(event *claudeEvent, observer Observer) {
	if event.Subtype != "init" {
		return
	}

	if len(event.Tools) > 0 {
		notef(observer, NoteInfo, "会话已启动，可用工具：%s",
			truncateTo(strings.Join(event.Tools, ", "), maxNoteRunes))

		return
	}

	notef(observer, NoteInfo, "会话已启动")
}

// narrateBlocks reports what the agent said and what it reached for. The tool
// calls are the useful part: they are the record of which searches ran and which
// pages were read to build the answer.
//
//nolint:gosmopolitan //informer is a chinese product; the notes speak the user's language.
func narrateBlocks(event *claudeEvent, observer Observer) {
	if event.Message == nil {
		return
	}

	for _, block := range event.Message.Content {
		switch block.Type {
		case blockText:
			if text := strings.TrimSpace(block.Text); text != "" {
				notef(observer, NoteInfo, "%s", truncateTo(text, maxNoteRunes))
			}
		case blockToolUse:
			notef(observer, NoteInfo, "调用 %s %s", block.Name, truncateTo(compactJSON(block.Input), maxNoteRunes))
		case blockToolResult:
			notef(observer, NoteInfo, "工具返回：%s", truncateTo(toolResultText(block.Content), maxToolResultRunes))
		}
	}
}

// toolResultText renders what a tool answered as readable text. The command line
// hands back either a plain string or a list of content blocks depending on the
// tool, so both shapes are unwrapped; anything else is narrated as it arrived.
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var text string

	err := json.Unmarshal(raw, &text)
	if err == nil {
		return text
	}

	var blocks []claudeBlock

	err = json.Unmarshal(raw, &blocks)
	if err != nil {
		return compactJSON(raw)
	}

	parts := make([]string, 0, len(blocks))

	for _, block := range blocks {
		if trimmed := strings.TrimSpace(block.Text); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	if len(parts) == 0 {
		return compactJSON(raw)
	}

	return strings.Join(parts, "\n")
}

// compactJSON renders a tool input on one line, falling back to the raw bytes
// when it is not an object this package can re-encode.
func compactJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var buffer bytes.Buffer

	err := json.Compact(&buffer, raw)
	if err != nil {
		return string(raw)
	}

	return buffer.String()
}
