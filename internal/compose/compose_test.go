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

// Every case here asserts on the chinese text informer ships - the tool answers
// an agent reads and the rules it is given - so the whole file quotes it.
//
//nolint:gosmopolitan //the assertions quote the shipped chinese text.
package compose_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/compose"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/mcp"
)

const (
	postRegex = `<a class="post" href="([^"]+)">([^<]+)</a>`
	titleExp  = "$2"
	urlExp    = "$1"
	firstPost = "第一篇文章"

	// noArticles is the refusal a configuration that parses nothing earns, and
	// the phrase the tool answers with.
	noArticles = "解析出 0 条"

	// dirFlag names the run directory on the tool server command line.
	dirFlag = "--dir"
)

// listingServer answers a two article html listing.
func listingServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
			<li><a class="post" href="/a.html">` + firstPost + `</a></li>
			<li><a class="post" href="/b.html">第二篇文章</a></li>
		`))
	}))
	t.Cleanup(server.Close)

	return server
}

// emptyFeedServer answers a well formed rss feed with no items in it. A parse
// that succeeds and yields nothing is a different outcome from one that failed,
// and it is the one a half working rule usually produces.
func emptyFeedServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>空的</title></channel></rss>`))
	}))
	t.Cleanup(server.Close)

	return server
}

// session builds a conversation whose proposals land in a temporary directory.
func session(t *testing.T) *compose.Session {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, compose.WriteSession(dir, &compose.Session{}))

	loaded, err := compose.ReadSession(dir)
	require.NoError(t, err)

	return loaded
}

// toolsByName indexes a session's tools so a test can call one by the name the
// agent would address it with.
func toolsByName(tools []mcp.Tool) map[string]mcp.Handler {
	byName := make(map[string]mcp.Handler, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool.Handler
	}

	return byName
}

// proposalOf reads what the conversation has settled on, failing the test when
// reading it errors.
func proposalOf(t *testing.T, dir string) *compose.Proposal {
	t.Helper()

	proposal, err := compose.ReadProposal(dir)
	require.NoError(t, err)

	return proposal
}

// The tools are the contract: look, try, and hand over. Nothing here reaches the
// database, and the one tool that records a proposal writes a file the parent
// re-checks before anyone sees a button.
func TestToolsOfferNoWayToSave(t *testing.T) {
	t.Parallel()

	names := make([]string, 0, 3)

	for _, tool := range compose.Tools(session(t)) {
		require.NotContains(t, tool.Name, "save")
		require.NotContains(t, tool.Name, "update")
		require.NotContains(t, tool.Name, "create")
		require.NotNil(t, tool.Handler)
		require.NotEmpty(t, tool.Description)

		names = append(names, tool.Name)
	}

	require.Equal(t, []string{"fetch_content", "try_parse", "propose_config"}, names)
}

// Every offered tool has to be pre-approved by name, or a headless run stalls on
// a permission prompt it has no way to answer. The web tools come along so the
// agent can go looking for a feed address it was not given.
func TestAllowedToolsCoversEveryOfferedToolAndTheWebOnes(t *testing.T) {
	t.Parallel()

	allowed := compose.AllowedTools()

	for _, tool := range compose.Tools(session(t)) {
		require.Contains(t, allowed, "mcp__"+compose.ServerName+"__"+tool.Name)
	}

	require.Contains(t, allowed, "WebSearch")
	require.Contains(t, allowed, "WebFetch")
}

// There is no source yet, so a trial has nothing to inherit and a child process
// is thrown away between turns. A call that carried only the field it wanted to
// change would silently parse an empty draft.
func TestTryParseCarriesTheWholeCandidateEveryTime(t *testing.T) {
	t.Parallel()

	server := listingServer(t)
	byName := toolsByName(compose.Tools(session(t)))

	whole, err := byName["try_parse"](context.Background(), json.RawMessage(`{
		"url": `+quote(server.URL)+`,
		"parse_type": "regex",
		"regex": `+quote(postRegex)+`,
		"title_exp": "$2",
		"url_exp": "$1"
	}`))
	require.NoError(t, err)
	require.Contains(t, whole, "解析成功，共 2 条")
	require.Contains(t, whole, firstPost)

	// the second call inherits nothing from the first: without its own url there
	// is no page to parse at all.
	partial, err := byName["try_parse"](context.Background(), json.RawMessage(`{"parse_type":"feed"}`))
	require.NoError(t, err)
	require.Contains(t, partial, "解析失败")
	require.NotContains(t, partial, firstPost)

	// the tool description has to say so, because nothing else can.
	require.Contains(t, descriptionOf(t, "try_parse"), "每次调用都必须给出完整配置")
}

// A fetch has no subscription to fall back on either, and saying so is more use
// to the agent than an error about an empty address.
func TestFetchContentRequiresAnAddress(t *testing.T) {
	t.Parallel()

	byName := toolsByName(compose.Tools(session(t)))

	_, err := byName["fetch_content"](context.Background(), json.RawMessage(`{}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "请在参数里给出 url")

	server := listingServer(t)

	window, err := byName["fetch_content"](context.Background(),
		json.RawMessage(`{"url":`+quote(server.URL)+`}`))
	require.NoError(t, err)
	require.Contains(t, window, firstPost)
}

// The proposal is recorded only when it really parses. This is the check that
// makes "解析出真正的文章" enforceable rather than advisory: the agent finds out
// while it can still fix the configuration.
func TestProposeConfigRecordsAWorkingConfiguration(t *testing.T) {
	t.Parallel()

	server := listingServer(t)
	current := session(t)
	byName := toolsByName(compose.Tools(current))

	answer, err := byName["propose_config"](context.Background(), json.RawMessage(`{
		"title": "测试站",
		"reason": "列表页用正则取标题和链接",
		"url": `+quote(server.URL)+`,
		"parse_type": "regex",
		"regex": `+quote(postRegex)+`,
		"title_exp": "$2",
		"url_exp": "$1"
	}`))
	require.NoError(t, err)
	require.Contains(t, answer, "已记录提案")

	proposal := proposalOf(t, current.Dir)
	require.NotNil(t, proposal)
	require.Equal(t, "测试站", proposal.Title)
	require.Equal(t, "列表页用正则取标题和链接", proposal.Reason)

	// what is recorded is the string the trial ran with, not one the model
	// retyped into its reply.
	candidate := proposal.Changes.Apply(&feed.Source{})
	require.Equal(t, postRegex, candidate.Regex)
	require.Equal(t, feed.ParseTypeRegex, candidate.ParseType)
	require.Equal(t, titleExp, candidate.TitleExp)
	require.Equal(t, urlExp, candidate.URLExp)
}

// A configuration that parses nothing is refused, and refusing it must leave no
// proposal behind: a stale one would turn into a save button for a source that
// never worked.
func TestProposeConfigRefusesAndRecordsNothing(t *testing.T) {
	t.Parallel()

	server := listingServer(t)
	emptyFeed := emptyFeedServer(t)

	cases := []struct {
		name      string
		arguments string
		refusal   string
	}{
		{
			name: "解析失败",
			arguments: `{"title":"测试站","url":` + quote(server.URL) + `,"parse_type":"regex",
				"regex":"<a class=\"missing\" href=\"([^\"]+)\">([^<]+)</a>","title_exp":"$2","url_exp":"$1"}`,
			refusal: "这套配置解析失败了",
		},
		{
			name:      noArticles,
			arguments: `{"title":"空的","url":` + quote(emptyFeed.URL) + `,"parse_type":"feed"}`,
			refusal:   noArticles,
		},
		{
			name: "全部指向同一个地址",
			arguments: `{"title":"测试站","url":` + quote(server.URL) + `,"parse_type":"regex",
				"regex":"<a class=\"post\" href=\"([^\"]+)\">([^<]+)</a>","title_exp":"$2","url_exp":"/only.html"}`,
			refusal: "指向同一个地址",
		},
		{
			name:      "没有标题",
			arguments: `{"url":` + quote(server.URL) + `,"parse_type":"feed"}`,
			refusal:   "没有给出 title",
		},
		{
			name:      "agent 类型没有提示词",
			arguments: `{"title":"测试站","parse_type":"agent"}`,
			refusal:   "agent_prompt 有意义，而它是空的",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := session(t)
			byName := toolsByName(compose.Tools(current))

			answer, err := byName["propose_config"](context.Background(), json.RawMessage(testCase.arguments))
			require.NoError(t, err)
			require.Contains(t, answer, "拒绝")
			require.Contains(t, answer, testCase.refusal)

			require.Nil(t, proposalOf(t, current.Dir))
		})
	}
}

// An agent candidate cannot be tried from inside an agent run, so it is taken on
// its word - and the answer has to say so, because the user is the one who ends
// up verifying it.
func TestProposeConfigAcceptsAnAgentCandidateWithoutATrial(t *testing.T) {
	t.Parallel()

	current := session(t)
	byName := toolsByName(compose.Tools(current))

	answer, err := byName["propose_config"](context.Background(), json.RawMessage(`{
		"title": "Go 语言周报",
		"parse_type": "agent",
		"agent_prompt": "搜索 Go 语言最近一周的技术文章，挑出最值得读的几篇"
	}`))
	require.NoError(t, err)
	require.Contains(t, answer, "无法在这里试跑")
	require.Contains(t, answer, "测试抓取")

	proposal := proposalOf(t, current.Dir)
	require.NotNil(t, proposal)
	require.Equal(t, feed.ParseTypeAgent, proposal.Changes.Apply(&feed.Source{}).ParseType)
}

// The conversation moves forward, so the newest proposal replaces the previous
// one. The earlier ones are still in the chat a person is reading.
func TestProposeConfigKeepsTheNewestProposal(t *testing.T) {
	t.Parallel()

	current := session(t)
	byName := toolsByName(compose.Tools(current))

	for _, title := range []string{"第一版", "第二版"} {
		_, err := byName["propose_config"](context.Background(), json.RawMessage(`{
			"title": `+quote(title)+`,
			"parse_type": "agent",
			"agent_prompt": "找文章"
		}`))
		require.NoError(t, err)
	}

	require.Equal(t, "第二版", proposalOf(t, current.Dir).Title)
}

// A conversation that has settled on nothing yet is the normal state of every
// turn that only asked a question.
func TestReadProposalAnswersNilBeforeAnyProposal(t *testing.T) {
	t.Parallel()

	require.Nil(t, proposalOf(t, t.TempDir()))
}

// The rules travel in the system prompt, restated every turn, so this is the one
// place the ordering of the parse types is stated at all.
func TestSystemRulesStateTheOrderAndTheDivisionOfLabour(t *testing.T) {
	t.Parallel()

	rules := compose.SystemRules()

	require.Contains(t, rules, "mcp__"+compose.ServerName+"__")
	require.Contains(t, rules, "必须按下面的顺序依次尝试")
	require.Contains(t, rules, "WebSearch / WebFetch 只能用来探索")
	require.Contains(t, rules, "任何候选配置都必须用 try_parse 试跑验证过")
	require.Contains(t, rules, "你没有保存配置的能力")
	require.Contains(t, rules, "agent 订阅每次定时抓取都要真的跑一次 AI")
	require.Contains(t, rules, "指向同一个地址，是失败不是成功")

	// every settable field is named somewhere in the rules or in the schema the
	// tools carry; the ones the rules explain by hand must not go stale.
	for _, name := range []string{"json_title_path", "json_url_path", "title_exp", "url_exp", "agent_prompt"} {
		require.Contains(t, rules, name)
	}
}

func TestOpeningPromptCarriesWhatTheUserSaid(t *testing.T) {
	t.Parallel()

	opening := compose.OpeningPrompt("  我想订阅阮一峰的博客  ")

	require.Contains(t, opening, "用户想新建一个订阅")
	require.Contains(t, opening, "我想订阅阮一峰的博客")
	require.Equal(t, "换一个地址试试", compose.TurnPrompt(" 换一个地址试试 "))
}

// ServeArgs is what every executable entry uses to decide whether it was
// launched as a tool server. It must not answer to the diagnosis sub command:
// the two carry different session documents.
func TestServeArgsRecognisesOnlyItsOwnInvocation(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()

	dir, ok := compose.ServeArgs([]string{compose.ServeCommand, dirFlag, runDir})
	require.True(t, ok)
	require.Equal(t, runDir, dir)

	for _, args := range [][]string{
		nil,
		{},
		{compose.ServeCommand},
		{compose.ServeCommand, dirFlag},
		{"mcp-diagnose", dirFlag, runDir},
		{"feed", "list"},
	} {
		_, found := compose.ServeArgs(args)
		require.False(t, found, args)
	}
}

// Serve is the whole chain the agent command line drives: a session directory
// in, an mcp conversation out.
func TestServeAnswersOverTheProtocol(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, compose.WriteSession(dir, &compose.Session{}))

	conversation := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"

	var out strings.Builder

	require.NoError(t, compose.Serve(context.Background(), dir, "test", strings.NewReader(conversation), &out))

	answers := out.String()
	require.Contains(t, answers, mcp.ProtocolVersion)
	require.Contains(t, answers, "propose_config")
	require.Contains(t, answers, "try_parse")
}

// A directory with no session document is not a conversation, and saying so is
// better than serving tools that read an empty draft.
func TestServeRefusesADirectoryWithoutASession(t *testing.T) {
	t.Parallel()

	err := compose.Serve(context.Background(), t.TempDir(), "test", strings.NewReader(""), &strings.Builder{})
	require.ErrorIs(t, err, compose.ErrNoSession)
}

// ServeStdio is the child process entry. What it has to get right beyond Serve
// is that stdout carries the protocol alone: informer logs a line for every
// fetch, and one of them landing in the stream is a parse error on the other
// side.
//
// It swaps the process wide os.Stdin, os.Stdout and logger output, so unlike
// every other case here it must not run alongside the others.
//
//nolint:paralleltest //swaps process wide stdio.
func TestServeStdioKeepsStdoutForTheProtocolAlone(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, compose.WriteSession(dir, &compose.Session{}))

	stdinRead, stdinWrite, err := os.Pipe()
	require.NoError(t, err)

	stdoutRead, stdoutWrite, err := os.Pipe()
	require.NoError(t, err)

	realIn, realOut := os.Stdin, os.Stdout

	os.Stdin, os.Stdout = stdinRead, stdoutWrite

	t.Cleanup(func() {
		os.Stdin, os.Stdout = realIn, realOut
	})

	go func() {
		_, _ = stdinWrite.WriteString(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")

		// a line the global logger emits while the session is open; it must not
		// reach the pipe stdout now points at.
		logger.Info("a fetch happened")

		_ = stdinWrite.Close()
	}()

	collected := make(chan string, 1)

	go func() {
		out, _ := io.ReadAll(stdoutRead)
		collected <- string(out)
	}()

	require.NoError(t, compose.ServeStdio(context.Background(), dir, "test"))
	require.NoError(t, stdoutWrite.Close())

	answered := <-collected
	require.Contains(t, answered, `"id":1`)
	require.NotContains(t, answered, "a fetch happened")
}

// quote renders a json string literal.
func quote(value string) string {
	// a string always marshals; the helper stays expression shaped so the
	// fixtures below read as the json they are.
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}

	return string(encoded)
}

// descriptionOf returns the description of one offered tool.
func descriptionOf(t *testing.T, name string) string {
	t.Helper()

	for _, tool := range compose.Tools(session(t)) {
		if tool.Name == name {
			return tool.Description
		}
	}

	t.Fatalf("no tool named %s", name)

	return ""
}
