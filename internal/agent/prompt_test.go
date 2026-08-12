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

package agent_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
)

// sampleURL is the one article link every parsing case in this package uses.
const sampleURL = "https://a.com/1"

// the contract informer appends is chinese, so the case that proves it is
// appended verbatim has to quote it.
//
//nolint:gosmopolitan //asserting on the shipped chinese contract text.
func TestBuildPromptKeepsInstructionAndAppendsContract(t *testing.T) {
	t.Parallel()

	prompt := agent.BuildPrompt("  找出今天的 Go 语言新闻  ", 5)

	require.True(t, strings.HasPrefix(prompt, "找出今天的 Go 语言新闻\n"))
	require.Contains(t, prompt, `{"items":[{"title":"文章标题","url":"文章链接"}]}`)
	require.Contains(t, prompt, "最多返回 5 条")
}

//nolint:gosmopolitan //asserting on the shipped chinese contract text.
func TestBuildPromptBoundsTheRequestedCount(t *testing.T) {
	t.Parallel()

	require.Contains(t, agent.BuildPrompt("x", 0), "最多返回 "+strconv.Itoa(agent.DefaultMaxItems)+" 条")
	require.Contains(t, agent.BuildPrompt("x", -3), "最多返回 "+strconv.Itoa(agent.DefaultMaxItems)+" 条")
	require.Contains(t, agent.BuildPrompt("x", 10_000), "最多返回 "+strconv.Itoa(agent.MaxItemsCap)+" 条")
}

func TestParseItemsAcceptsTheContractShape(t *testing.T) {
	t.Parallel()

	items, err := agent.ParseItems(`{"items":[{"title":"a","url":"` + sampleURL + `"}]}`)

	require.NoError(t, err)
	require.Equal(t, []agent.Item{{Title: "a", URL: sampleURL}}, items)
}

// a real model answers in the language of the prompt, so the wrapper this case
// has to survive is chinese prose.
//
//nolint:gosmopolitan //modelling a chinese model answer.
func TestParseItemsAcceptsAWrappedOrBareAnswer(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"code fence":     "```json\n{\"items\":[{\"title\":\"a\",\"url\":\"https://a.com/1\"}]}\n```",
		"bare array":     `[{"title":"a","url":"` + sampleURL + `"}]`,
		"chatty wrapper": "这是结果:\n{\"items\":[{\"title\":\"a\",\"url\":\"https://a.com/1\"}]}\n希望有用。",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			items, err := agent.ParseItems(raw)

			require.NoError(t, err)
			require.Equal(t, []agent.Item{{Title: "a", URL: sampleURL}}, items)
		})
	}
}

func TestParseItemsDropsUnusableEntries(t *testing.T) {
	t.Parallel()

	raw := `{"items":[
		{"title":"keep","url":"https://a.com/1"},
		{"title":"","url":"https://a.com/2"},
		{"title":"relative","url":"/news/3"},
		{"title":"scheme","url":"ftp://a.com/4"},
		{"title":"duplicate","url":"https://a.com/1"},
		{"title":" trimmed ","url":" https://a.com/5 "}
	]}`

	items, err := agent.ParseItems(raw)

	require.NoError(t, err)
	require.Equal(t, []agent.Item{
		{Title: "keep", URL: sampleURL},
		{Title: "trimmed", URL: "https://a.com/5"},
	}, items)
}

func TestParseItemsReturnsEmptyForAnEmptyAnswer(t *testing.T) {
	t.Parallel()

	items, err := agent.ParseItems(`{"items":[]}`)

	require.NoError(t, err)
	require.Empty(t, items)
}

//nolint:gosmopolitan //modelling a chinese model answer.
func TestParseItemsRejectsOutputWithoutJSON(t *testing.T) {
	t.Parallel()

	_, err := agent.ParseItems("抱歉，我没有找到任何结果。")

	require.ErrorIs(t, err, agent.ErrNoJSONOutput)
}

func TestParseItemsRejectsBrokenJSON(t *testing.T) {
	t.Parallel()

	_, err := agent.ParseItems(`{"items":[{"title":"a","url":}]}`)

	require.Error(t, err)
	require.NotErrorIs(t, err, agent.ErrNoJSONOutput)
}
