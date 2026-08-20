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
// an agent reads and the prompt it is given - so the whole file quotes it.
//
//nolint:gosmopolitan //the assertions quote the shipped chinese text.
package diagnose_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/diagnose"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/mcp"
)

// Fixtures repeated across the cases below, named so a change to one of them is
// a change to every case that relies on it.
const (
	urlExp   = "$1"
	titleExp = "$2"
	noMatch  = "no match"
	blogName = "博客"
	someBlog = "某博客"
	someRule = "abc"
	sameRule = "same"
)

func strp(v string) *string { return &v }

func boolp(v bool) *bool { return &v }

func TestChangesApplyLeavesTheStoredSourceAlone(t *testing.T) {
	t.Parallel()

	source := &feed.Source{ID: 7, Regex: "old", TitleExp: urlExp, Redirect: false}
	changes := &diagnose.Changes{Regex: strp("new"), Redirect: boolp(true)}

	patched := changes.Apply(source)

	require.Equal(t, "new", patched.Regex)
	require.True(t, patched.Redirect)
	require.Equal(t, urlExp, patched.TitleExp, "an unmentioned field keeps its value")

	require.Equal(t, "old", source.Regex, "the snapshot handed in must not be edited")
	require.False(t, source.Redirect)
}

// A field the agent restated without changing must not show up as an edit: a
// model that echoes the whole configuration back would otherwise look like it
// rewrote everything.
func TestDiffDropsUnchangedFields(t *testing.T) {
	t.Parallel()

	source := &feed.Source{Regex: sameRule, URLExp: urlExp}
	changes := &diagnose.Changes{Regex: strp(sameRule), URLExp: strp(titleExp)}

	diff := changes.Diff(source)

	require.Len(t, diff, 1)
	require.Equal(t, "url_exp", diff[0].Field)
	require.Equal(t, urlExp, diff[0].Old)
	require.Equal(t, titleExp, diff[0].New)
}

func TestEffectiveKeepsOnlyRealEdits(t *testing.T) {
	t.Parallel()

	source := &feed.Source{Regex: sameRule, URLExp: urlExp, IsJSON: false}
	changes := &diagnose.Changes{Regex: strp(sameRule), URLExp: strp(titleExp), IsJSON: boolp(true)}

	effective := changes.Effective(source)

	require.NotNil(t, effective)
	require.Nil(t, effective.Regex, "an unchanged field is dropped")
	require.NotNil(t, effective.URLExp)
	require.Equal(t, titleExp, *effective.URLExp)
	require.NotNil(t, effective.IsJSON)
	require.True(t, *effective.IsJSON)

	require.Nil(t, (&diagnose.Changes{Regex: strp(sameRule)}).Effective(source),
		"a proposal that changes nothing is no proposal")
}

// Clearing a field is a real repair - a source that should become a plain feed
// again - so an empty string must not read as "not mentioned".
func TestEmptyStringIsAnEditNotAnOmission(t *testing.T) {
	t.Parallel()

	source := &feed.Source{Regex: "broken"}
	changes := &diagnose.Changes{Regex: strp("")}

	require.False(t, changes.IsEmpty())
	require.Empty(t, changes.Apply(source).Regex)
	require.Len(t, changes.Diff(source), 1)
}

func TestParseReportReadsAFencedAnswer(t *testing.T) {
	t.Parallel()

	report, err := diagnose.ParseReport("解析完成：\n```json\n" +
		`{"fixed":true,"diagnosis":"页面结构变了","changes":{"regex":"a(b)c"},"advice":""}` +
		"\n```\n")
	require.NoError(t, err)

	require.True(t, report.Fixed)
	require.Equal(t, "页面结构变了", report.Diagnosis)
	require.NotNil(t, report.Changes)
	require.Equal(t, "a(b)c", *report.Changes.Regex)
}

// "Cannot fix" is a legitimate outcome and has to survive parsing intact: the
// advice is the whole value of a run that repaired nothing.
func TestParseReportKeepsAnUnfixableAnswer(t *testing.T) {
	t.Parallel()

	report, err := diagnose.ParseReport(
		`{"fixed":false,"diagnosis":"站点已关闭","changes":null,"advice":"建议删除这个订阅"}`)
	require.NoError(t, err)

	require.False(t, report.Fixed)
	require.Nil(t, report.Changes)
	require.Equal(t, "建议删除这个订阅", report.Advice)
}

// A model that claims success while proposing nothing is claiming a fix it did
// not make; the empty change set decides, not the flag.
func TestParseReportRefusesFixedWithoutChanges(t *testing.T) {
	t.Parallel()

	report, err := diagnose.ParseReport(`{"fixed":true,"diagnosis":"没事","changes":{}}`)
	require.NoError(t, err)

	require.False(t, report.Fixed)
	require.Nil(t, report.Changes)
}

func TestParseReportRefusesProse(t *testing.T) {
	t.Parallel()

	_, err := diagnose.ParseReport("我看了一下，这个正则应该改成 a(b)c。")
	require.ErrorIs(t, err, diagnose.ErrNoReport)
}

func TestSessionRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	session := &diagnose.Session{
		SourceID:    3,
		Source:      &feed.Source{ID: 3, Title: blogName, URL: "https://example.com"},
		StoredError: noMatch,
	}

	require.NoError(t, diagnose.WriteSession(dir, session))

	loaded, err := diagnose.ReadSession(dir)
	require.NoError(t, err)
	require.Equal(t, int64(3), loaded.SourceID)
	require.Equal(t, "博客", loaded.Source.Title)
	require.Equal(t, noMatch, loaded.StoredError)

	_, err = diagnose.ReadSession(t.TempDir())
	require.ErrorIs(t, err, diagnose.ErrNoSession)
}

func TestWriteMCPConfigNamesTheServer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	path, err := diagnose.WriteMCPConfig(dir, "/usr/local/bin/informer", "mcp-diagnose", "--dir", dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, diagnose.MCPConfigFileName), path)

	var config diagnose.MCPServerConfig

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &config))

	entry, found := config.MCPServers[diagnose.ServerName]
	require.True(t, found)
	require.Equal(t, "/usr/local/bin/informer", entry.Command)
	require.Equal(t, []string{"mcp-diagnose", "--dir", dir}, entry.Args)
}

// Every offered tool has to be pre-approved by name, or a headless run stalls on
// a permission prompt it has no way to answer.
//
// The web tools are allowed alongside them so a diagnosis can go looking - for
// where a feed moved to, for whether the site announced a redesign - instead of
// only staring at one document. Judging is a different matter, and the rule that
// the verdict rests on fetch_content's bytes lives in the prompt, which the case
// below asserts.
func TestAllowedToolsCoversEveryOfferedTool(t *testing.T) {
	t.Parallel()

	allowed := diagnose.AllowedTools()

	for _, tool := range diagnose.Tools(&diagnose.Session{Source: &feed.Source{}}) {
		require.Contains(t, allowed, "mcp__"+diagnose.ServerName+"__"+tool.Name)
	}

	require.Contains(t, allowed, "WebSearch")
	require.Contains(t, allowed, "WebFetch")
}

// The tools are the contract: a diagnosis may look and try, and has no way at
// all to save. This is the guarantee the whole feature rests on.
func TestToolsOfferNoWayToSave(t *testing.T) {
	t.Parallel()

	for _, tool := range diagnose.Tools(&diagnose.Session{Source: &feed.Source{}}) {
		require.NotContains(t, tool.Name, "save")
		require.NotContains(t, tool.Name, "update")
		require.NotContains(t, tool.Name, "write")
		require.NotNil(t, tool.Handler)
		require.NotEmpty(t, tool.Description)
	}
}

// The end to end shape of a repair: the page changed, the stored regex matches
// nothing, and a candidate regex tried through the tool parses it correctly.
func TestTryParseRunsACandidateAgainstTheLivePage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
			<li><a class="post" href="/a.html">第一篇文章</a></li>
			<li><a class="post" href="/b.html">第二篇文章</a></li>
		`))
	}))
	defer server.Close()

	session := &diagnose.Session{
		SourceID: 1,
		Source: &feed.Source{
			ID:        1,
			Title:     "测试站",
			URL:       server.URL,
			ParseType: feed.ParseTypeRegex,
			// the regex the page used to answer to, and no longer does.
			Regex:    `<a class="entry" href="([^"]+)">([^<]+)</a>`,
			URLExp:   urlExp,
			TitleExp: titleExp,
		},
		StoredError: noMatch,
		FreshError:  noMatch,
	}

	byName := toolsByName(diagnose.Tools(session))

	current, err := byName["try_parse"](context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Contains(t, current, "解析失败", "the stored configuration is the broken one")

	fixed, err := byName["try_parse"](context.Background(),
		json.RawMessage(`{"regex":"<a class=\"post\" href=\"([^\"]+)\">([^<]+)</a>"}`))
	require.NoError(t, err)
	require.Contains(t, fixed, "解析成功，共 2 条")
	require.Contains(t, fixed, "第一篇文章")
	require.Contains(t, fixed, "regex:")

	// the snapshot must survive a trial run untouched: a trial that edited it
	// would make every later trial run against the previous guess.
	require.Equal(t, `<a class="entry" href="([^"]+)">([^<]+)</a>`, session.Source.Regex)
}

func TestFetchContentSlicesAndSearches(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", 200) + "第一篇文章" + strings.Repeat("y", 200)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	byName := toolsByName(diagnose.Tools(&diagnose.Session{
		Source: &feed.Source{URL: server.URL, ParseType: feed.ParseTypeRegex},
	}))

	window, err := byName["fetch_content"](context.Background(), json.RawMessage(`{"offset":0,"length":50}`))
	require.NoError(t, err)
	require.Contains(t, window, "[0, 50)")
	require.Contains(t, window, "offset=50")

	hit, err := byName["fetch_content"](context.Background(), json.RawMessage(`{"contains":"第一篇文章"}`))
	require.NoError(t, err)
	require.Contains(t, hit, "第 1 处")
	require.Contains(t, hit, "第一篇文章")

	miss, err := byName["fetch_content"](context.Background(), json.RawMessage(`{"contains":"不存在的标题"}`))
	require.NoError(t, err)
	require.Contains(t, miss, "没有找到")

	// no arguments at all is a legal call and means the defaults.
	first, err := byName["fetch_content"](context.Background(), nil)
	require.NoError(t, err)
	require.Contains(t, first, "[0, ")
}

func TestGetSourceStatesTheFailure(t *testing.T) {
	t.Parallel()

	byName := toolsByName(diagnose.Tools(&diagnose.Session{
		SourceID:    9,
		Source:      &feed.Source{ID: 9, Title: blogName, Regex: someRule, ParseType: feed.ParseTypeRegex},
		StoredError: noMatch,
		FreshError:  noMatch,
	}))

	answer, err := byName["get_source"](context.Background(), nil)
	require.NoError(t, err)
	require.Contains(t, answer, "订阅 #9")
	require.Contains(t, answer, `regex = "`+someRule+`"`)
	require.Contains(t, answer, noMatch)
}

// A source that starts working again between the failure and the diagnosis must
// be reported as such, not repaired: changing a working source is a regression.
func TestGetSourceSaysWhenTheRetrySucceeded(t *testing.T) {
	t.Parallel()

	byName := toolsByName(diagnose.Tools(&diagnose.Session{
		SourceID:    9,
		Source:      &feed.Source{ID: 9, Title: blogName},
		StoredError: "timeout",
	}))

	answer, err := byName["get_source"](context.Background(), nil)
	require.NoError(t, err)
	require.Contains(t, answer, "间歇性故障")
}

func TestBuildPromptStatesTheRulesTheToolsEnforce(t *testing.T) {
	t.Parallel()

	prompt := diagnose.BuildPrompt(&diagnose.Session{
		SourceID: 4,
		Source:   &feed.Source{ID: 4, Title: someBlog},
	})

	require.Contains(t, prompt, "订阅 #4「"+someBlog+"」")
	require.Contains(t, prompt, "mcp__"+diagnose.ServerName+"__")

	// every repairable field is named, so what the agent may touch and what it
	// is told about cannot drift apart.
	for _, name := range diagnose.RepairableFields() {
		require.Contains(t, prompt, name)
	}

	require.Contains(t, prompt, "你没有保存配置的能力")
	require.Contains(t, prompt, `{"fixed":`)

	// the web tools are allowed by AllowedTools, so the only thing keeping the
	// verdict on informer's own bytes is this sentence.
	require.Contains(t, prompt, "WebSearch / WebFetch 只能用来探索")
	require.Contains(t, prompt, "任何结论都必须以 fetch_content 取到的字节为准")
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

// Serve is what the child process is: a session directory in, an mcp
// conversation out. This is the one test that exercises the whole chain the
// agent command line actually drives.
func TestServeAnswersOverTheProtocol(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, diagnose.WriteSession(dir, &diagnose.Session{
		SourceID:    11,
		Source:      &feed.Source{ID: 11, Title: someBlog, Regex: someRule},
		StoredError: noMatch,
		FreshError:  noMatch,
	}))

	var out strings.Builder

	conversation := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_source","arguments":{}}}`,
	}, "\n") + "\n"

	require.NoError(t, diagnose.Serve(context.Background(), dir, "test", strings.NewReader(conversation), &out))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 3, "the notification is not answered")

	var listed struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}

	require.NoError(t, json.Unmarshal([]byte(lines[1]), &listed))
	require.Len(t, listed.Result.Tools, 3)

	require.Contains(t, lines[2], someBlog)

	_, err := os.Stat(filepath.Join(dir, "feed.db"))
	require.True(t, os.IsNotExist(err), "the tool server must never open a database")
}

// ServeArgs is what every executable entry uses to decide whether it was
// launched by a person or by the agent command line of a diagnosis. Getting it
// wrong either way is severe: a false positive opens no window, a false negative
// opens one in the middle of a json-rpc stream.
func TestServeArgsRecognisesOnlyTheToolServerInvocation(t *testing.T) {
	t.Parallel()

	const (
		runDir  = "/tmp/run"
		dirFlag = "--dir"
	)

	dir, ok := diagnose.ServeArgs([]string{diagnose.ServeCommand, dirFlag, runDir})
	require.True(t, ok)
	require.Equal(t, runDir, dir)

	for _, args := range [][]string{
		nil,
		{},
		{"version"},
		{diagnose.ServeCommand},
		{diagnose.ServeCommand, dirFlag},
		{diagnose.ServeCommand, "--home", runDir},
		{"feed", dirFlag, runDir},
		{dirFlag, runDir, diagnose.ServeCommand},
	} {
		_, found := diagnose.ServeArgs(args)
		require.False(t, found, "must not be mistaken for tool server mode: %v", args)
	}
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

	require.NoError(t, diagnose.WriteSession(dir, &diagnose.Session{
		SourceID: 5,
		Source:   &feed.Source{ID: 5, Title: blogName},
	}))

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

	require.NoError(t, diagnose.ServeStdio(context.Background(), dir, "test"))
	require.NoError(t, stdoutWrite.Close())

	answered := <-collected

	for line := range strings.SplitSeq(strings.TrimSpace(answered), "\n") {
		var frame map[string]any

		require.NoError(t, json.Unmarshal([]byte(line), &frame),
			"every line on stdout has to be a protocol frame, got %q", line)
	}

	require.Contains(t, answered, `"id":1`)
	require.NotContains(t, answered, "a fetch happened")
}

// An unreadable run directory has to be reported rather than served empty: a
// tool server answering about a source it never loaded would diagnose nothing.
func TestServeRefusesADirectoryWithoutASession(t *testing.T) {
	t.Parallel()

	err := diagnose.Serve(context.Background(), t.TempDir(), "test",
		strings.NewReader(""), io.Discard)
	require.ErrorIs(t, err, diagnose.ErrNoSession)
}

// A run directory that cannot be created or written is reported, not ignored: a
// session the child cannot read is a diagnosis that cannot start.
func TestSessionAndConfigWritesReportAnUnusablePath(t *testing.T) {
	t.Parallel()

	// a path under a regular file can never become a directory.
	blocked := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocked, []byte("x"), 0o600))

	under := filepath.Join(blocked, "run")

	require.Error(t, diagnose.WriteSession(under, &diagnose.Session{Source: &feed.Source{}}))

	_, err := diagnose.WriteMCPConfig(under, "/bin/informer", diagnose.ServeCommand)
	require.Error(t, err)

	require.Error(t, diagnose.WriteSession(t.TempDir(), nil), "there is no session to write")
}

// A session document that exists but describes nothing is refused too: the
// tools would otherwise dereference a source that was never there.
func TestReadSessionRefusesADocumentWithoutASource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, diagnose.SessionFileName),
		[]byte(`{"source_id":1}`), 0o600))

	_, err := diagnose.ReadSession(dir)
	require.ErrorIs(t, err, diagnose.ErrNoSession)

	require.NoError(t, os.WriteFile(filepath.Join(dir, diagnose.SessionFileName),
		[]byte(`not json`), 0o600))

	_, err = diagnose.ReadSession(dir)
	require.ErrorIs(t, err, diagnose.ErrNoSession)
}

// An agent source would start another agent from inside the one already
// running. The nesting is refused rather than run.
func TestTryParseRefusesToNestAnotherAgent(t *testing.T) {
	t.Parallel()

	byName := toolsByName(diagnose.Tools(&diagnose.Session{
		Source: &feed.Source{ID: 2, Title: someBlog, ParseType: feed.ParseTypeAgent, AgentPrompt: "找文章"},
	}))

	answer, err := byName["try_parse"](context.Background(), json.RawMessage(`{"agent_prompt":"换个说法"}`))
	require.NoError(t, err)
	require.Contains(t, answer, "不支持试跑")
	require.NotContains(t, answer, "解析成功")
}

// Bad arguments are a tool level failure the model can correct, so they come
// back as an error the server turns into an isError result.
func TestToolsReportUnusableArguments(t *testing.T) {
	t.Parallel()

	byName := toolsByName(diagnose.Tools(&diagnose.Session{Source: &feed.Source{}}))

	_, err := byName["try_parse"](context.Background(), json.RawMessage(`{"regex":123}`))
	require.Error(t, err)

	_, err = byName["fetch_content"](context.Background(), json.RawMessage(`["not an object"]`))
	require.Error(t, err)
}

// A source that cannot be reached fails the same way every time; the failure is
// remembered so a retry loop does not hammer a dead host.
func TestFetchContentRemembersAFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable := server.URL
	server.Close()

	byName := toolsByName(diagnose.Tools(&diagnose.Session{
		Source: &feed.Source{URL: unreachable, ParseType: feed.ParseTypeRegex},
	}))

	_, first := byName["fetch_content"](context.Background(), nil)
	require.Error(t, first)

	_, second := byName["fetch_content"](context.Background(), nil)
	require.Error(t, second)
	require.Equal(t, first.Error(), second.Error())
}

// An empty document is stated as such rather than returned as an empty window,
// which would read as "the page has nothing in it here".
func TestFetchContentReportsAnEmptyDocument(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no charset, so the body is handed back as the zero bytes it is rather
		// than failing in the decoder.
		w.Header().Set("Content-Type", "text/html")
	}))
	defer server.Close()

	byName := toolsByName(diagnose.Tools(&diagnose.Session{
		Source: &feed.Source{URL: server.URL, ParseType: feed.ParseTypeRegex},
	}))

	_, err := byName["fetch_content"](context.Background(), nil)
	require.ErrorIs(t, err, diagnose.ErrNoContent)
}

// The window bounds: an oversized request is clamped, and an offset past the end
// says so instead of answering with nothing.
func TestFetchContentBoundsTheWindow(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(strings.Repeat("x", 120)))
	}))
	defer server.Close()

	byName := toolsByName(diagnose.Tools(&diagnose.Session{
		Source: &feed.Source{URL: server.URL, ParseType: feed.ParseTypeRegex},
	}))

	whole, err := byName["fetch_content"](context.Background(),
		json.RawMessage(`{"offset":-5,"length":100000}`))
	require.NoError(t, err)
	require.Contains(t, whole, "[0, 120)")
	require.NotContains(t, whole, "继续读", "the whole document was returned")

	past, err := byName["fetch_content"](context.Background(), json.RawMessage(`{"offset":500}`))
	require.NoError(t, err)
	require.Contains(t, past, "越过结尾")
}

// A candidate address is fetched plainly, which is what lets the agent test the
// guess that the site moved.
func TestFetchContentFollowsACandidateAddress(t *testing.T) {
	t.Parallel()

	moved := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("新的地址上的内容"))
	}))
	defer moved.Close()

	byName := toolsByName(diagnose.Tools(&diagnose.Session{
		Source: &feed.Source{URL: "http://127.0.0.1:1/gone", ParseType: feed.ParseTypeRegex},
	}))

	answer, err := byName["fetch_content"](context.Background(),
		json.RawMessage(`{"url":"`+moved.URL+`","refresh":true}`))
	require.NoError(t, err)
	require.Contains(t, answer, "新的地址上的内容")
}

// A model that answers with prose around its json still answered, and a fence
// with nothing usable in it did not.
func TestParseReportRecoversAWrappedAnswer(t *testing.T) {
	t.Parallel()

	report, err := diagnose.ParseReport("```\n" +
		`{"fixed":false,"diagnosis":"需要登录","advice":"换一个地址"}` + "\n```")
	require.NoError(t, err)
	require.Equal(t, "需要登录", report.Diagnosis)

	for _, answer := range []string{"```", "```json\n没有 json\n```", "", "{", "}{"} {
		_, err = diagnose.ParseReport(answer)
		require.ErrorIs(t, err, diagnose.ErrNoReport, "answer: %q", answer)
	}

	_, err = diagnose.ParseReport(`{"fixed": "not a bool"}`)
	require.Error(t, err, "a json document of the wrong shape is a parse failure")
}
