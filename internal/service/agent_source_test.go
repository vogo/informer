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
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// stubAgent writes an executable stand-in for the agent command line, records the
// prompt it was called with, and configures the service to run it. It lets the
// whole agent path - configuration, prompt assembly, exec, parsing - be exercised
// without a real agent or a network.
func stubAgent(t *testing.T, svc *service.Service, answer string) string {
	t.Helper()

	if runtime.GOOS == windowsGOOS {
		t.Skip("the stub agent is a posix shell script")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "stub-agent")
	promptFile := filepath.Join(dir, "prompt")

	// the prompt is the last argument the runner passes before the flags, so the
	// script records every argument and the assertions look for the instruction.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + promptFile + "\n" +
		"cat <<'STUB_EOF'\n" +
		`{"type":"result","subtype":"success","is_error":false,"result":` + jsonString(answer) + "}\n" +
		"STUB_EOF\n"

	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700)) //nolint:gosec //test helper binary.
	require.NoError(t, svc.SaveAgentConfig(&agent.Config{Command: binary}))

	return promptFile
}

// jsonString quotes a value as a json string literal.
func jsonString(value string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}

	return string(raw)
}

// agentPrompt is the plain instruction the stored agent source carries. It is
// written in english so the linter that guards against hardcoded chinese in
// source files has nothing to complain about; a real user writes their own.
const agentPrompt = "find today's articles about the Go programming language"

// unknownAgentProvider is a provider name no build can drive.
const unknownAgentProvider = "gemini"

// agentSource builds a stored agent source: an instruction and no address at all.
func agentSource() *feed.Source {
	return &feed.Source{
		Title:       "agent source",
		Weight:      60,
		ParseType:   feed.ParseTypeAgent,
		AgentPrompt: agentPrompt,
	}
}

func TestCreateAgentSourceNeedsNoURL(t *testing.T) {
	svc := newService(t)

	source := agentSource()
	require.NoError(t, svc.CreateSource(source))

	stored, err := svc.GetSource(source.ID)
	require.NoError(t, err)
	assert.Equal(t, feed.ParseTypeAgent, stored.ResolveParseType())
	assert.Equal(t, agentPrompt, stored.AgentPrompt)
	assert.Empty(t, stored.URL)
	assert.True(t, stored.Enabled)
}

func TestAgentSourceIsRefusedWithoutAPrompt(t *testing.T) {
	svc := newService(t)

	source := agentSource()
	source.AgentPrompt = "   "

	require.ErrorIs(t, svc.CreateSource(source), service.ErrInvalidArgument)
}

func TestAgentSourceIsRefusedWithAnUnknownProvider(t *testing.T) {
	svc := newService(t)

	source := agentSource()
	source.AgentProvider = unknownAgentProvider

	require.ErrorIs(t, svc.CreateSource(source), service.ErrInvalidArgument)
}

func TestFetchingSourceStillNeedsAnAddress(t *testing.T) {
	svc := newService(t)

	err := svc.CreateSource(&feed.Source{Title: "a source with neither url nor curl"})
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}

// the appended contract is chinese, and proving it reached the agent means
// quoting it.
//
//nolint:gosmopolitan //asserting on the shipped chinese contract text.
func TestPreviewAgentSourceRunsTheConfiguredAgent(t *testing.T) {
	svc := newService(t)
	promptFile := stubAgent(t, svc, `{"items":[
		{"title":"agent one","url":"https://example.com/agent-one"},
		{"title":"agent two","url":"https://example.com/agent-two"}]}`)

	source := agentSource()
	source.MaxFetchNum = 7
	require.NoError(t, svc.CreateSource(source))

	articles, err := svc.Preview(source.ID)
	require.NoError(t, err)
	require.Len(t, articles, 2)
	assert.Equal(t, "agent one", articles[0].Title)
	assert.Equal(t, "https://example.com/agent-one", articles[0].URL)
	assert.Equal(t, source.ID, articles[0].SourceID)
	assert.Equal(t, int64(60), articles[0].Weight)

	// the user only wrote the instruction; the json contract and the item limit
	// are what informer adds on top of it.
	prompt, err := os.ReadFile(promptFile)
	require.NoError(t, err)
	assert.Contains(t, string(prompt), agentPrompt)
	assert.Contains(t, string(prompt), `{"items":[{"title":"文章标题","url":"文章链接"}]}`)
	assert.Contains(t, string(prompt), "最多返回 7 条")

	// a preview writes nothing at all, an agent source included.
	before := snapshotDB(t, svc)
	_, err = svc.Preview(source.ID)
	require.NoError(t, err)
	assert.Equal(t, before, snapshotDB(t, svc))
}

func TestPreviewAgentSourceReportsAFailedRun(t *testing.T) {
	svc := newService(t)
	stubAgent(t, svc, "I could not find anything.")

	source := agentSource()
	require.NoError(t, svc.CreateSource(source))

	_, err := svc.Preview(source.ID)
	require.ErrorIs(t, err, agent.ErrNoJSONOutput)
}

func TestAgentSourcesAreFilteredByTheirParseType(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)

	require.NoError(t, svc.CreateSource(agentSource()))
	require.NoError(t, svc.CreateSource(feedSource(server)))

	agents, err := svc.AllSources(service.SourceQuery{ParseType: feed.ParseTypeAgent})
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "agent source", agents[0].Title)

	// a source that predates the parse type column is derived, and deriving can
	// never name the agent parser, so it stays out of this bucket.
	feeds, err := svc.AllSources(service.SourceQuery{ParseType: feed.ParseTypeFeed})
	require.NoError(t, err)
	require.Len(t, feeds, 1)
	assert.Equal(t, "atom source", feeds[0].Title)
}
