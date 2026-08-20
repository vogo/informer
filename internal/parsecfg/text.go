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

package parsecfg

import "strings"

// codeFence is the marker a model wraps code in.
const codeFence = "```"

// StripCodeFence removes the ``` wrapper a model may put around its json.
func StripCodeFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, codeFence) {
		return text
	}

	newline := strings.IndexByte(text, '\n')
	if newline < 0 {
		return ""
	}

	text = text[newline+1:]

	if closing := strings.LastIndex(text, codeFence); closing >= 0 {
		text = text[:closing]
	}

	return strings.TrimSpace(text)
}

// ExtractJSONObject returns the outermost json object inside the text, so a
// stray sentence before or after the answer does not cost the whole run.
//
// It is only safe on an answer that is meant to be json and nothing else. On
// prose that happens to contain braces - a regex with {2,} in it, a sentence
// about items[] - the span it returns starts in the prose, and the caller gets
// either a parse error or, worse, a successful parse of the wrong thing. Use
// LastFencedJSON when the answer is a person facing reply.
func ExtractJSONObject(text string) string {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return ""
	}

	end := strings.LastIndexByte(text, '}')
	if end <= start {
		return ""
	}

	return text[start : end+1]
}

// LastFencedJSON returns the body of the last fenced json block of a reply, or
// an empty string when the reply carries none.
//
// A chat turn is prose with a block appended, so the block has to be found by
// its fence rather than by its braces: the prose above it routinely contains
// both. Only a block that is fenced as json (or fenced with no language at all)
// and opens with an object is taken.
func LastFencedJSON(text string) string {
	var (
		found  string
		offset int
	)

	for {
		open := strings.Index(text[offset:], codeFence)
		if open < 0 {
			return found
		}

		open += offset

		newline := strings.IndexByte(text[open:], '\n')
		if newline < 0 {
			return found
		}

		info := strings.TrimSpace(text[open+len(codeFence) : open+newline])
		bodyStart := open + newline + 1

		closing := strings.Index(text[bodyStart:], codeFence)
		if closing < 0 {
			return found
		}

		body := strings.TrimSpace(text[bodyStart : bodyStart+closing])
		if (info == "" || strings.EqualFold(info, "json")) && strings.HasPrefix(body, "{") {
			found = body
		}

		offset = bodyStart + closing + len(codeFence)
	}
}

// Truncate shortens text to at most limit runes, cutting on rune boundaries so a
// chinese answer is never sliced into invalid utf-8.
func Truncate(text string, limit int) string {
	trimmed := strings.TrimSpace(text)

	runes := []rune(trimmed)
	if len(runes) <= limit {
		return trimmed
	}

	return string(runes[:limit]) + "..."
}
