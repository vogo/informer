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
	"encoding/json"
	"fmt"
	"strings"
)

// outputContract is appended to every instruction. It is the only part of the
// prompt informer writes itself, and it exists so a source author never has to
// spell the json shape out by hand.
//
//nolint:gosmopolitan //informer is a chinese product; the contract speaks the user's language.
const outputContract = `
---
以上是本次任务。以下是 informer 追加的输出格式要求，必须严格遵守：

1. 最终答复只输出一个 JSON 对象，不要输出任何解释、前言、总结或 Markdown 代码块标记。
2. JSON 对象的结构固定为：
{"items":[{"title":"文章标题","url":"文章链接"}]}
3. title 是非空字符串；url 必须是以 http:// 或 https:// 开头的完整可访问链接，不要输出相对路径。
4. 最多返回 %d 条，按重要程度从高到低排列，不要重复同一个链接。
5. 没有任何结果时，输出 {"items":[]}，不要编造链接。`

// BuildPrompt assembles the text handed to the agent: the user's own instruction
// first, then the output contract. maxItems of zero falls back to DefaultMaxItems
// and any larger request is capped, so one source can never ask for an unbounded
// answer.
func BuildPrompt(userPrompt string, maxItems int) string {
	limit := maxItems
	if limit <= 0 {
		limit = DefaultMaxItems
	}

	if limit > MaxItemsCap {
		limit = MaxItemsCap
	}

	return strings.TrimSpace(userPrompt) + "\n" + fmt.Sprintf(outputContract, limit)
}

// itemsDocument is the object shape the contract asks for.
type itemsDocument struct {
	Items []Item `json:"items"`
}

// ParseItems reads the articles out of an agent answer.
//
// The contract asks for a bare {"items":[...]} object, but a model that wraps it
// in a code fence, or answers with the array alone, is still understood: the
// alternative is throwing away a perfectly good result over punctuation. Entries
// without a title or without an absolute http url are dropped rather than stored,
// and a repeated url is kept only once.
func ParseItems(raw string) ([]Item, error) {
	document := extractJSON(stripCodeFence(raw))
	if document == "" {
		return nil, fmt.Errorf("%w: %q", ErrNoJSONOutput, truncate(raw))
	}

	var parsed itemsDocument

	// a bare array is the one deviation from the contract that carries the same
	// information, so it is read rather than refused.
	if strings.HasPrefix(document, "[") {
		err := json.Unmarshal([]byte(document), &parsed.Items)
		if err != nil {
			return nil, fmt.Errorf("parse agent json array: %w", err)
		}

		return cleanItems(parsed.Items), nil
	}

	err := json.Unmarshal([]byte(document), &parsed)
	if err != nil {
		return nil, fmt.Errorf("parse agent json object: %w", err)
	}

	return cleanItems(parsed.Items), nil
}

// cleanItems keeps the usable entries in their original order.
func cleanItems(items []Item) []Item {
	cleaned := make([]Item, 0, len(items))
	seen := make(map[string]bool, len(items))

	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		link := strings.TrimSpace(item.URL)

		if title == "" || !isAbsoluteHTTPURL(link) || seen[link] {
			continue
		}

		seen[link] = true

		cleaned = append(cleaned, Item{Title: title, URL: link})
	}

	return cleaned
}

// isAbsoluteHTTPURL reports whether the link is one informer can store and open.
// A relative path is refused instead of guessed at: an agent source has no base
// url to resolve it against.
func isAbsoluteHTTPURL(link string) bool {
	return strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://")
}

// stripCodeFence removes the ``` wrapper a model may put around its json.
func stripCodeFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "```") {
		return text
	}

	// drop the opening fence line, which may carry a language tag.
	if newline := strings.IndexByte(text, '\n'); newline >= 0 {
		text = text[newline+1:]
	} else {
		return ""
	}

	if closing := strings.LastIndex(text, "```"); closing >= 0 {
		text = text[:closing]
	}

	return strings.TrimSpace(text)
}

// extractJSON returns the outermost json document inside the text, so a stray
// sentence before or after the answer does not cost the whole result.
func extractJSON(text string) string {
	start := strings.IndexAny(text, "{[")
	if start < 0 {
		return ""
	}

	closing := byte('}')
	if text[start] == '[' {
		closing = ']'
	}

	end := strings.LastIndexByte(text, closing)
	if end <= start {
		return ""
	}

	return text[start : end+1]
}

// maxDiagnosticRunes bounds the raw output quoted in an error message.
const maxDiagnosticRunes = 300

// truncate shortens output quoted into an error so a runaway answer cannot
// bury the actual failure. It cuts on rune boundaries, so a chinese answer is
// never sliced into invalid utf-8.
func truncate(text string) string {
	trimmed := strings.TrimSpace(text)

	runes := []rune(trimmed)
	if len(runes) <= maxDiagnosticRunes {
		return trimmed
	}

	return string(runes[:maxDiagnosticRunes]) + "..."
}
