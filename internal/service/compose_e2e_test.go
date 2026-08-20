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

package service_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/runlog"
	"github.com/vogo/informer/internal/service"
)

// unconfiguredListing serves a plain html listing with no feed anywhere: three
// posts wrapped in a nav bar and a footer, which is what the conversation has to
// tell apart.
//
//nolint:gosmopolitan //informer is a chinese product; the fixtures speak the user's language.
func unconfiguredListing(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>
			<nav><a href="/about">关于</a><a href="/rss">订阅</a></nav>
			<ul class="post-list">
			<li class="post-item"><a class="post-link" href="/p/1001">Go 1.25 泛型改进实践</a></li>
			<li class="post-item"><a class="post-link" href="/p/1002">用 eBPF 定位线上抖动</a></li>
			<li class="post-item"><a class="post-link" href="/p/1003">SQLite 并发写入的真相</a></li>
			</ul>
			<footer><a href="/contact">联系我们</a></footer>
			</body></html>`))
	}))
	t.Cleanup(server.Close)

	return server
}

// TestComposeSourceEndToEnd drives the whole composing feature against a real
// agent: a listing page nobody has configured yet, a person describing it in one
// sentence, and a conversation that has to read the page, try a candidate and
// hand back a configuration informer can verify.
//
// It is the only test that proves the parts connect - the mcp server the agent
// reaches is this very test binary, launched through TestMain, and the
// conversation is resumed through the real command line. What is asserted is
// deliberately not "the agent wrote this exact regex", which is not
// reproducible, but the contract around it: a proposal is offered only when
// informer's own parse produced articles, and saving it stores a working source.
//
//nolint:gosmopolitan //informer is a chinese product; the fixtures speak the user's language.
func TestComposeSourceEndToEnd(t *testing.T) {
	if os.Getenv(agentE2EEnv) == "" {
		t.Skipf("set %s=1 to run the real agent end to end", agentE2EEnv)
	}

	server := unconfiguredListing(t)
	svc := newService(t)

	sink := runlog.FuncSink(func(entry runlog.Entry) {
		t.Logf("[%s] %s", entry.Level, entry.Text)
	})

	sessionID, err := svc.StartCompose()
	require.NoError(t, err)

	t.Cleanup(func() { svc.CloseCompose(sessionID) })

	reply, err := svc.ComposeChat(t.Context(), sessionID,
		"我想订阅 "+server.URL+" 这个页面上的文章列表，帮我配好。", sink)
	require.NoError(t, err)

	t.Logf("agent: %s", reply.Message)
	require.NotEmpty(t, reply.Message, "a turn always says something")
	require.Equal(t, 1, reply.Turns)

	if reply.Proposal == nil {
		// asking a question first is legitimate; the second turn is where the
		// resumed conversation has to remember what page it was looking at.
		reply, err = svc.ComposeChat(t.Context(), sessionID, "就用这个页面，尽量不要用 agent 类型。", sink)
		require.NoError(t, err)

		t.Logf("agent: %s", reply.Message)
		require.Equal(t, 2, reply.Turns)
	}

	proposal := reply.Proposal
	require.NotNil(t, proposal, "two turns should be enough for a plain listing page")

	t.Logf("proposal %q (%s): %s", proposal.Title, proposal.ParseType, proposal.Reason)

	require.NotEmpty(t, proposal.Title)
	require.NotEmpty(t, proposal.Fields)
	require.NotEqual(t, feed.ParseTypeAgent, proposal.ParseType,
		"a plain html listing must not fall through to an agent source")

	require.True(t, proposal.Savable, "informer's own parse is what the save button follows")
	require.True(t, proposal.Verification.Ran)
	require.Empty(t, proposal.Verification.Error)
	require.Equal(t, 3, proposal.Verification.ArticleCount,
		"the three posts, not the nav bar and not the footer")

	requireComposedSource(t, svc, proposal, server.URL)
}

// requireComposedSource asserts what pressing save does: the configuration
// lands, and the stored source parses the three posts.
func requireComposedSource(t *testing.T, svc *service.Service,
	proposal *service.ComposeProposal, pageURL string,
) {
	t.Helper()

	created, err := svc.CreateSourceFromCompose(proposal.Changes, proposal.Title, 0)
	require.NoError(t, err)

	stored, err := svc.GetSource(created.ID)
	require.NoError(t, err)
	require.Equal(t, proposal.Title, stored.Title)
	require.Equal(t, pageURL, stored.URL)
	require.True(t, stored.Enabled)

	articles, err := svc.PreviewSource(stored)
	require.NoError(t, err)
	require.Len(t, articles, 3)

	for _, article := range articles {
		t.Logf("parsed: %s | %s", article.Title, article.URL)
		require.NotEmpty(t, article.Title)
	}
}
