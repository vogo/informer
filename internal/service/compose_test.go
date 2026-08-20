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
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// postsRegex matches the fixture listing below.
const postsRegex = `<a class="post" href="([^"]+)">([^<]+)</a>`

// composeStub is a stand-in agent command line for a composing conversation.
//
// Beyond answering, it can drop a proposal document into the run directory,
// which is what the real mcp child does when the agent calls propose_config.
// The run directory is not passed to the stub directly - it is the directory of
// the --mcp-config path - so finding it is also a check that the flag really
// went out.
type composeStub struct {
	record   string
	proposal string
}

// args returns every argument the stub was last invoked with.
func (s composeStub) args(t *testing.T) []string {
	t.Helper()

	data, err := os.ReadFile(s.record)
	require.NoError(t, err)

	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// propose makes every later turn write this document as its proposal.
func (s composeStub) propose(t *testing.T, document string) {
	t.Helper()

	require.NoError(t, os.WriteFile(s.proposal, []byte(document), 0o600))
}

// stubComposeAgent installs a stand-in agent command line on the service.
func stubComposeAgent(t *testing.T, svc *service.Service, answer string) composeStub {
	t.Helper()

	if runtime.GOOS == windowsGOOS {
		t.Skip("the stub agent is a posix shell script")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "stub-agent")
	stub := composeStub{record: filepath.Join(dir, "args"), proposal: filepath.Join(dir, "proposal-source")}

	script := "#!/bin/sh\n" +
		"prev=''\nmcp=''\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"--mcp-config\" ]; then mcp=\"$arg\"; fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"printf '%s\\n' \"$@\" > " + stub.record + "\n" +
		"if [ -n \"$mcp\" ] && [ -f " + stub.proposal + " ]; then\n" +
		"  cp " + stub.proposal + " \"$(dirname \"$mcp\")/proposal.json\"\n" +
		"fi\n" +
		"cat <<'STUB_EOF'\n" +
		`{"type":"result","subtype":"success","is_error":false,"result":` + jsonString(answer) + "}\n" +
		"STUB_EOF\n"

	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700)) //nolint:gosec //test helper binary.
	require.NoError(t, svc.SaveAgentConfig(&agent.Config{Command: binary}))

	return stub
}

// postsServer answers a two article html listing.
func postsServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`
			<li><a class="post" href="/a.html">First post</a></li>
			<li><a class="post" href="/b.html">Second post</a></li>
		`))
	}))
	t.Cleanup(server.Close)

	return server
}

// regexProposal is the document a working conversation would have recorded.
func regexProposal(url, rule string) string {
	return `{"title":"Test blog","reason":"the listing page yields titles and links",` +
		`"changes":{"url":` + jsonString(url) + `,"parse_type":"regex","regex":` + jsonString(rule) +
		`,"title_exp":"$2","url_exp":"$1"}}`
}

// A turn that only asked a question is the normal case, and it must not produce
// a save button.
func TestComposeChatAnswersWithoutProposingAnything(t *testing.T) {
	svc := newService(t)
	stubComposeAgent(t, svc, "which blog do you want to follow?")

	id, err := svc.StartCompose()
	require.NoError(t, err)
	require.NotEmpty(t, id)

	reply, err := svc.ComposeChat(t.Context(), id, "I want to follow a blog", nil)
	require.NoError(t, err)

	require.Equal(t, id, reply.SessionID)
	require.Equal(t, "which blog do you want to follow?", reply.Message)
	require.Equal(t, 1, reply.Turns)
	require.Nil(t, reply.Proposal)
}

// Every turn carries the rules, the tools and the session name, because the
// command line remembers the conversation but not how it was invoked.
func TestComposeChatConfiguresEveryTurn(t *testing.T) {
	svc := newService(t)
	stub := stubComposeAgent(t, svc, "let me look")

	id, err := svc.StartCompose()
	require.NoError(t, err)

	_, err = svc.ComposeChat(t.Context(), id, "https://example.com", nil)
	require.NoError(t, err)

	first := strings.Join(stub.args(t), "\n")
	require.Contains(t, first, "--append-system-prompt")
	require.Contains(t, first, "--mcp-config")
	require.Contains(t, first, "--session-id")
	require.NotContains(t, first, "--resume")
	require.Contains(t, first, "mcp__informer__propose_config")
	require.Contains(t, first, "WebSearch")

	_, err = svc.ComposeChat(t.Context(), id, "try the feed", nil)
	require.NoError(t, err)

	second := strings.Join(stub.args(t), "\n")
	require.Contains(t, second, "--resume")
	require.Contains(t, second, "--append-system-prompt")
	require.Contains(t, second, "--mcp-config")
}

// The proposal is informer's to judge. The agent recorded it; what the person
// is shown is what happened when informer parsed with it, here, just now.
func TestComposeChatVerifiesARecordedProposal(t *testing.T) {
	svc := newService(t)
	server := postsServer(t)
	stub := stubComposeAgent(t, svc, "found it")
	stub.propose(t, regexProposal(server.URL, postsRegex))

	id, err := svc.StartCompose()
	require.NoError(t, err)

	reply, err := svc.ComposeChat(t.Context(), id, "https://example.com", nil)
	require.NoError(t, err)

	proposal := reply.Proposal
	require.NotNil(t, proposal)
	require.Equal(t, "Test blog", proposal.Title)
	require.Equal(t, feed.ParseTypeRegex, proposal.ParseType)
	require.True(t, proposal.Savable)

	require.True(t, proposal.Verification.Ran)
	require.Equal(t, 2, proposal.Verification.ArticleCount)
	require.Equal(t, "First post", proposal.Verification.Samples[0].Title)

	// the fields are rendered against an empty draft, so every one of them is
	// new; there is no previous value for a subscription that does not exist.
	byField := map[string]string{}
	for _, field := range proposal.Fields {
		byField[field.Field] = field.New
		require.Empty(t, field.Old)
	}

	require.Equal(t, postsRegex, byField["regex"])
	require.Equal(t, server.URL, byField["url"])
}

// A configuration that does not parse is reported as such and is not savable:
// the agent's word never reaches the button.
func TestComposeChatRefusesAProposalThatDoesNotParse(t *testing.T) {
	svc := newService(t)
	server := postsServer(t)
	stub := stubComposeAgent(t, svc, "this should work")
	stub.propose(t, regexProposal(server.URL, `<a class="missing" href="([^"]+)">([^<]+)</a>`))

	id, err := svc.StartCompose()
	require.NoError(t, err)

	reply, err := svc.ComposeChat(t.Context(), id, "https://example.com", nil)
	require.NoError(t, err)

	require.NotNil(t, reply.Proposal)
	require.False(t, reply.Proposal.Savable)
	require.True(t, reply.Proposal.Verification.Ran)
	require.NotEmpty(t, reply.Proposal.Verification.Error)
}

// The proposal document outlives the turn that wrote it, so a conversation that
// went on talking must not put the same card in the chat again.
func TestComposeChatReportsAProposalOnlyOnce(t *testing.T) {
	svc := newService(t)
	server := postsServer(t)
	stub := stubComposeAgent(t, svc, "found it")
	stub.propose(t, regexProposal(server.URL, postsRegex))

	id, err := svc.StartCompose()
	require.NoError(t, err)

	first, err := svc.ComposeChat(t.Context(), id, "https://example.com", nil)
	require.NoError(t, err)
	require.NotNil(t, first.Proposal)

	second, err := svc.ComposeChat(t.Context(), id, "looks good", nil)
	require.NoError(t, err)
	require.Nil(t, second.Proposal)

	// a different configuration is a different card.
	stub.propose(t, regexProposal(server.URL+"/other", postsRegex))

	third, err := svc.ComposeChat(t.Context(), id, "try the other page", nil)
	require.NoError(t, err)
	require.NotNil(t, third.Proposal)
}

// An agent candidate cannot be tried from inside an agent run. Saying "not
// verified" is honest; saying "verification failed" would not be.
func TestComposeChatMarksAnAgentProposalUnverified(t *testing.T) {
	svc := newService(t)
	stub := stubComposeAgent(t, svc, "only an agent can do this one")
	stub.propose(t, `{"title":"Weekly Go","reason":"the site renders in the browser",`+
		`"changes":{"parse_type":"agent","agent_prompt":"find recent go articles"}}`)

	id, err := svc.StartCompose()
	require.NoError(t, err)

	reply, err := svc.ComposeChat(t.Context(), id, "https://example.com", nil)
	require.NoError(t, err)

	proposal := reply.Proposal
	require.NotNil(t, proposal)
	require.Equal(t, feed.ParseTypeAgent, proposal.ParseType)
	require.False(t, proposal.Verification.Ran)
	require.Empty(t, proposal.Verification.Error)
	require.NotEmpty(t, proposal.Verification.Note)

	// it is still savable: an unverifiable configuration is not a broken one,
	// and the note tells the user to test fetch it after saving.
	require.True(t, proposal.Savable)
}

// propose_config is the contract, but an answer that put the configuration in a
// fenced block instead would otherwise reach a conclusion with no way to save
// it. The fallback is verified by exactly the same parse.
func TestComposeChatFallsBackToAFencedBlock(t *testing.T) {
	svc := newService(t)
	server := postsServer(t)

	answer := "I looked at the page and the listing uses `a.post`, so a regex works.\n\n" +
		"```json\n" + regexProposal(server.URL, postsRegex) + "\n```"

	stubComposeAgent(t, svc, answer)

	id, err := svc.StartCompose()
	require.NoError(t, err)

	reply, err := svc.ComposeChat(t.Context(), id, "https://example.com", nil)
	require.NoError(t, err)

	require.NotNil(t, reply.Proposal)
	require.True(t, reply.Proposal.Savable)
	require.Equal(t, 2, reply.Proposal.Verification.ArticleCount)

	// the block stays in the message: it is what the agent showed the user.
	require.Contains(t, reply.Message, "a regex works")
}

// The modal is singular, so a second conversation means the first one is gone.
// Ending it is what keeps a force quit window from leaking a run directory.
func TestStartComposeReplacesThePreviousConversation(t *testing.T) {
	svc := newService(t)
	stubComposeAgent(t, svc, "hello")

	first, err := svc.StartCompose()
	require.NoError(t, err)

	second, err := svc.StartCompose()
	require.NoError(t, err)
	require.NotEqual(t, first, second)

	_, err = svc.ComposeChat(t.Context(), first, "still there?", nil)
	require.ErrorIs(t, err, service.ErrNotFound)

	_, err = svc.ComposeChat(t.Context(), second, "hello", nil)
	require.NoError(t, err)
}

func TestCloseComposeEndsTheConversation(t *testing.T) {
	svc := newService(t)
	stubComposeAgent(t, svc, "hello")

	id, err := svc.StartCompose()
	require.NoError(t, err)

	svc.CloseCompose(id)

	_, err = svc.ComposeChat(t.Context(), id, "still there?", nil)
	require.ErrorIs(t, err, service.ErrNotFound)

	// closing one that is already gone is not an error: the modal closes for
	// many reasons and the page calls this on all of them.
	svc.CloseCompose(id)
	svc.CloseCompose("never-existed")
}

func TestComposeChatRefusesAnEmptyMessage(t *testing.T) {
	svc := newService(t)
	stubComposeAgent(t, svc, "hello")

	id, err := svc.StartCompose()
	require.NoError(t, err)

	_, err = svc.ComposeChat(t.Context(), id, "   ", nil)
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

// Saving goes through the same validation every other created source does. A
// proposal is not trusted just because informer verified it a minute ago.
func TestCreateSourceFromComposeStoresAndValidates(t *testing.T) {
	svc := newService(t)
	server := postsServer(t)
	stub := stubComposeAgent(t, svc, "found it")
	stub.propose(t, regexProposal(server.URL, postsRegex))

	id, err := svc.StartCompose()
	require.NoError(t, err)

	reply, err := svc.ComposeChat(t.Context(), id, "https://example.com", nil)
	require.NoError(t, err)

	created, err := svc.CreateSourceFromCompose(reply.Proposal.Changes, "  My blog  ", 0)
	require.NoError(t, err)

	stored, err := svc.GetSource(created.ID)
	require.NoError(t, err)
	require.Equal(t, "My blog", stored.Title)
	require.Equal(t, server.URL, stored.URL)
	require.Equal(t, postsRegex, stored.Regex)
	require.Equal(t, feed.ParseTypeRegex, stored.ParseType)
	require.True(t, stored.Enabled)
	require.Equal(t, int64(feed.DefaultCategoryID), stored.CategoryID)

	_, err = svc.CreateSourceFromCompose(reply.Proposal.Changes, "   ", 0)
	require.ErrorIs(t, err, service.ErrInvalidArgument)

	_, err = svc.CreateSourceFromCompose(nil, "My blog", 0)
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}
