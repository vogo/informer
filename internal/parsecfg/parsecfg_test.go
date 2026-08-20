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

// The assertions quote the chinese text the tools hand an agent.
//
//nolint:gosmopolitan //the assertions quote the shipped chinese text.
package parsecfg_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/parsecfg"
)

// The rule the fixture page below answers to, and the group references that go
// with it.
const (
	postRegex = `<a class="post" href="([^"]+)">([^<]+)</a>`
	titleExp  = "$2"
)

// A chat reply is prose with a block appended. Braces in the prose above it -
// a regex quantifier, a sentence about items[] - are exactly what made reading
// the answer as "everything between the first { and the last }" unusable.
func TestLastFencedJSONIgnoresBracesInTheProse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want string
	}{
		{
			name: "prose with braces above the block",
			text: "我用的正则是 `\\d{4}`，路径写成 data/items[] 这种形式。\n\n" +
				"```json\n{\"ready\":true}\n```",
			want: `{"ready":true}`,
		},
		{
			name: "the last block wins",
			text: "先看这个例子：\n```json\n{\"first\":1}\n```\n最终配置是：\n```json\n{\"second\":2}\n```",
			want: `{"second":2}`,
		},
		{
			name: "a fence with no language still counts",
			text: "结果：\n```\n{\"bare\":true}\n```",
			want: `{"bare":true}`,
		},
		{
			name: "a block that is not an object is not an answer",
			text: "```json\n[1,2,3]\n```",
			want: "",
		},
		{
			name: "prose alone carries nothing",
			text: "这个站点的正则大概是 <a href=\"([^\"]+)\">，我还要再试一次。",
			want: "",
		},
		{
			name: "an unclosed fence carries nothing",
			text: "```json\n{\"half\":true}",
			want: "",
		},
		{
			name: "a shell block is not an answer",
			text: "```sh\n{ echo hi; }\n```",
			want: "",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, testCase.want, parsecfg.LastFencedJSON(testCase.text))
		})
	}
}

// A rule that matched the wrapper around the listing yields many rows and one
// address, which reads as success until this is looked at.
func TestTrialReportsRowsThatAllPointAtOneAddress(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
			<li><a class="post" href="/a.html">第一篇文章</a></li>
			<li><a class="post" href="/b.html">第二篇文章</a></li>
		`))
	}))
	defer server.Close()

	wrapped := parsecfg.Trial(&feed.Source{
		URL:       server.URL,
		ParseType: feed.ParseTypeRegex,
		Regex:     postRegex,
		TitleExp:  titleExp,
		URLExp:    "/only.html",
	})

	require.True(t, wrapped.OK(), "it parses, which is the whole trap")
	require.Len(t, wrapped.Articles, 2)
	require.Equal(t, 1, wrapped.DistinctURLs())

	var out strings.Builder

	parsecfg.WriteTrial(&out, wrapped)
	require.Contains(t, out.String(), "指向同一个地址")

	proper := parsecfg.Trial(&feed.Source{
		URL:       server.URL,
		ParseType: feed.ParseTypeRegex,
		Regex:     postRegex,
		TitleExp:  titleExp,
		URLExp:    "$1",
	})

	require.Equal(t, 2, proper.DistinctURLs())
	require.NotContains(t, trialText(proper), "指向同一个地址")
}

// An agent candidate would start another agent process from inside this one, so
// it is refused rather than run - and the refusal has to be visible as its own
// outcome, not as a failure.
func TestTrialRefusesToNestAnAgentRun(t *testing.T) {
	t.Parallel()

	result := parsecfg.Trial(&feed.Source{ParseType: feed.ParseTypeAgent, AgentPrompt: "找文章"})

	require.True(t, result.Skipped)
	require.False(t, result.OK())
	require.NoError(t, result.Err)
	require.Contains(t, trialText(result), "不支持试跑")
}

// Describe renders every column, including the empty ones: "this field is empty"
// is often the answer a tool is being asked for.
func TestDescribeRendersEveryColumn(t *testing.T) {
	t.Parallel()

	values := parsecfg.Describe(&feed.Source{Regex: "rule", Redirect: true})

	byField := make(map[string]string, len(values))
	for _, value := range values {
		byField[value.Field] = value.Text
	}

	require.Len(t, byField, len(parsecfg.Fields()))
	require.Equal(t, `"rule"`, byField[parsecfg.FieldRegex])
	require.Equal(t, `""`, byField[parsecfg.FieldURL])
	require.Equal(t, "true", byField[parsecfg.FieldRedirect])
	require.Equal(t, "false", byField[parsecfg.FieldIsJSON])
}

func trialText(result parsecfg.TrialResult) string {
	var out strings.Builder

	parsecfg.WriteTrial(&out, result)

	return out.String()
}
