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

package resultprocess_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/resultprocess"
)

const (
	firstTitle  = "first"
	secondTitle = "second"
	firstURL    = "https://example.com/one"
	secondURL   = "https://example.com/two"
)

func TestParseValidatesEveryCandidate(t *testing.T) {
	t.Parallel()
	articles := []*feed.Article{{ID: 1, Title: firstTitle, URL: firstURL}, {ID: 2, Title: secondTitle, URL: secondURL}}
	for name, raw := range map[string]string{
		"invalid JSON":       "bad answer",
		"unknown ID":         `{"action":"publish","summary":"digest","articles":[{"id":3,"reason":"useful"}],"rejected":[{"id":1,"reason":"skip"},{"id":2,"reason":"skip"}]}`,
		"duplicate ID":       `{"action":"publish","summary":"digest","articles":[{"id":1,"reason":"useful"},{"id":1,"reason":"again"}],"rejected":[{"id":2,"reason":"skip"}]}`,
		"missing decision":   `{"action":"publish","summary":"digest","articles":[{"id":1,"reason":"useful"}]}`,
		"empty summary":      `{"action":"skip","summary":" ","articles":[],"rejected":[{"id":1,"reason":"skip"},{"id":2,"reason":"skip"}]}`,
		"skip with articles": `{"action":"skip","summary":"nothing","articles":[{"id":1,"reason":"useful"}],"rejected":[{"id":2,"reason":"skip"}]}`,
		"empty publish":      `{"action":"publish","summary":"digest","articles":[],"rejected":[{"id":1,"reason":"skip"},{"id":2,"reason":"skip"}]}`,
		"invented link":      `{"action":"publish","summary":"https://invented.example","articles":[{"id":1,"reason":"useful"}],"rejected":[{"id":2,"reason":"skip"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			report, err := resultprocess.Parse(raw, articles, time.Now())
			require.ErrorIs(t, err, resultprocess.ErrInvalidResult)
			assert.Equal(t, raw, report.Raw)
			assert.Empty(t, report.Content)
		})
	}
}

func TestParseRendersOnlySelectedRealLinks(t *testing.T) {
	t.Parallel()
	articles := []*feed.Article{{ID: 1, Title: firstTitle, URL: "https://example.com/one"}, {ID: 2, Title: secondTitle, URL: secondURL}}
	raw := `{"action":"publish","summary":"digest","articles":[{"id":2,"reason":"best"}],"rejected":[{"id":1,"reason":"low value"}],"content":"untrusted content","raw":"untrusted raw"}`
	report, err := resultprocess.Parse(raw, articles, time.Now())
	require.NoError(t, err)
	assert.Contains(t, report.Content, "digest")
	assert.Contains(t, report.Content, "https://example.com/two")
	assert.NotContains(t, report.Content, "https://example.com/one")
	assert.NotContains(t, report.Content, "untrusted")
	assert.Equal(t, raw, report.Raw)
	assert.Equal(t, []*feed.Article{articles[1]}, report.Selected(articles))
}

func TestParseSkipAndFencedJSON(t *testing.T) {
	t.Parallel()
	report, err := resultprocess.Parse("```json\n"+`{"action":"skip","summary":"nothing qualifies","articles":[],"rejected":[]}`+"\n```", nil, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "skip", report.Action)
	assert.Empty(t, report.Content)
}
