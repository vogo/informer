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
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
)

const (
	// windowsGOOS is the one platform without the posix shell the stand-in agent needs.
	windowsGOOS = "windows"

	// testInstruction stands in for whatever a source author would write.
	testInstruction = "find today's news"
)

// fakeAgent is the stand-in command line one case runs.
type fakeAgent struct {
	// binary is the executable to configure as the agent command.
	binary string

	// record is the file the script writes its arguments and environment to.
	record string
}

// fakeClaude writes an executable stand-in for the claude command line. The
// script records the arguments and the anthropic environment it was called with,
// then prints the given stdout and exits with the given code, which lets the exec
// path be exercised without a real agent.
func fakeClaude(t *testing.T, stdout string, exitCode int) fakeAgent {
	t.Helper()

	if runtime.GOOS == windowsGOOS {
		t.Skip("the fake agent is a posix shell script")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-claude")
	record := filepath.Join(dir, "record")

	script := "#!/bin/sh\n" +
		"{\n" +
		"  for arg in \"$@\"; do printf 'arg:%s\\n' \"$arg\"; done\n" +
		"  printf 'env:ANTHROPIC_BASE_URL=%s\\n' \"$ANTHROPIC_BASE_URL\"\n" +
		"  printf 'env:ANTHROPIC_AUTH_TOKEN=%s\\n' \"$ANTHROPIC_AUTH_TOKEN\"\n" +
		"  printf 'env:ANTHROPIC_API_KEY=%s\\n' \"$ANTHROPIC_API_KEY\"\n" +
		"  printf 'env:HTTP_PROXY=%s\\n' \"$HTTP_PROXY\"\n" +
		"  printf 'env:HTTPS_PROXY=%s\\n' \"$HTTPS_PROXY\"\n" +
		"  printf 'env:ALL_PROXY=%s\\n' \"$ALL_PROXY\"\n" +
		"} > " + record + "\n" +
		"cat <<'INFORMER_EOF'\n" + stdout + "\nINFORMER_EOF\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"

	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700)) //nolint:gosec //test helper binary.

	return fakeAgent{binary: binary, record: record}
}

func envelope(result string) string {
	return `{"type":"result","subtype":"success","is_error":false,"result":` + quote(result) + `}`
}

func quote(value string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func TestRunParsesTheAgentAnswer(t *testing.T) {
	t.Parallel()

	stub := fakeClaude(t, envelope(`{"items":[{"title":"a","url":"`+sampleURL+`"}]}`), 0)

	result, err := agent.Run(t.Context(), &agent.Config{Command: stub.binary}, testInstruction, 3, nil)

	require.NoError(t, err)
	require.Equal(t, []agent.Item{{Title: "a", URL: sampleURL}}, result.Items)
}

func TestRunPassesConfigurationToTheCommandLine(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	stub := fakeClaude(t, envelope(`{"items":[]}`), 0)

	_, err := agent.Run(t.Context(), &agent.Config{
		Command:      stub.binary,
		BaseURL:      "https://proxy.example.com",
		APIKey:       "secret-key",
		Model:        "claude-sonnet-5",
		AllowedTools: "WebFetch",
		WorkDir:      workDir,
		HTTPProxy:    "http://127.0.0.1:7890",
	}, testInstruction, 3, nil)
	require.NoError(t, err)

	raw, err := os.ReadFile(stub.record)
	require.NoError(t, err)

	call := string(raw)
	require.Contains(t, call, "arg:--print\n")
	require.Contains(t, call, "arg:--output-format\narg:stream-json\n")
	require.Contains(t, call, "arg:--verbose\n")
	require.Contains(t, call, "arg:--model\narg:claude-sonnet-5\n")
	require.Contains(t, call, "arg:--tools\narg:WebFetch\n")
	require.Contains(t, call, "arg:--allowedTools\narg:WebFetch\n")
	require.Contains(t, call, "env:ANTHROPIC_BASE_URL=https://proxy.example.com\n")
	require.Contains(t, call, "env:ANTHROPIC_AUTH_TOKEN=secret-key\n")
	require.Contains(t, call, "env:ANTHROPIC_API_KEY=secret-key\n")
	require.Contains(t, call, "env:HTTP_PROXY=http://127.0.0.1:7890\n")
	require.Contains(t, call, "env:HTTPS_PROXY=http://127.0.0.1:7890\n")
	require.Contains(t, call, "env:ALL_PROXY=http://127.0.0.1:7890\n")
	require.Contains(t, call, testInstruction)
}

func TestRunLeavesTheMachineCredentialsAloneWhenUnconfigured(t *testing.T) {
	stub := fakeClaude(t, envelope(`{"items":[]}`), 0)

	t.Setenv("ANTHROPIC_BASE_URL", "https://machine.example.com")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "machine-token")

	_, err := agent.Run(t.Context(), &agent.Config{Command: stub.binary}, testInstruction, 3, nil)
	require.NoError(t, err)

	raw, err := os.ReadFile(stub.record)
	require.NoError(t, err)

	require.Contains(t, string(raw), "env:ANTHROPIC_BASE_URL=https://machine.example.com\n")
	require.Contains(t, string(raw), "env:ANTHROPIC_AUTH_TOKEN=machine-token\n")
}

func TestRunReportsACommandFailure(t *testing.T) {
	t.Parallel()

	stub := fakeClaude(t, "boom", 1)

	_, err := agent.Run(t.Context(), &agent.Config{Command: stub.binary}, testInstruction, 3, nil)

	require.ErrorIs(t, err, agent.ErrAgentFailed)
}

func TestRunReportsAnErrorEnvelope(t *testing.T) {
	t.Parallel()

	stub := fakeClaude(t, `{"type":"result","subtype":"error_max_turns","is_error":true,"result":"too many turns"}`, 0)

	_, err := agent.Run(t.Context(), &agent.Config{Command: stub.binary}, testInstruction, 3, nil)

	require.ErrorIs(t, err, agent.ErrAgentFailed)
	require.Contains(t, err.Error(), "error_max_turns")
}

func TestRunKeepsTheRawAnswerWhenParsingFails(t *testing.T) {
	t.Parallel()

	stub := fakeClaude(t, envelope("I could not find anything."), 0)

	result, err := agent.Run(t.Context(), &agent.Config{Command: stub.binary}, testInstruction, 3, nil)

	require.ErrorIs(t, err, agent.ErrNoJSONOutput)
	require.NotNil(t, result)
	require.Equal(t, "I could not find anything.", result.Raw)
}

func TestRunRefusesAnEmptyPrompt(t *testing.T) {
	t.Parallel()

	_, err := agent.Run(t.Context(), agent.DefaultConfig(), "   ", 3, nil)

	require.ErrorIs(t, err, agent.ErrEmptyPrompt)
}

func TestRunRefusesAProviderThisBuildCannotDrive(t *testing.T) {
	t.Parallel()

	for _, provider := range []string{agent.ProviderCodex, "gemini"} {
		_, err := agent.Run(t.Context(), &agent.Config{Provider: provider}, testInstruction, 3, nil)

		require.ErrorIs(t, err, agent.ErrUnsupportedProvider, provider)
	}
}

func TestRunHonoursTheConfiguredTimeout(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("the fake agent is a posix shell script")
	}

	dir := t.TempDir()
	slow := filepath.Join(dir, "slow-claude")
	require.NoError(t, os.WriteFile(slow, []byte("#!/bin/sh\nsleep 30\n"), 0o700)) //nolint:gosec //test helper binary.

	// the configured timeout floor is well above what a test may wait for, so the
	// caller's own deadline is what stops this run; either way the agent is killed
	// instead of blocking the fetch loop forever.
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	t.Cleanup(cancel)

	_, err := agent.Run(ctx, &agent.Config{Command: slow}, testInstruction, 3, nil)

	require.ErrorIs(t, err, agent.ErrAgentFailed)
}

func TestNormalizedFillsDefaultsAndClampsTheTimeout(t *testing.T) {
	t.Parallel()

	filled := (&agent.Config{}).Normalized()
	require.Equal(t, agent.DefaultProvider, filled.Provider)
	require.Equal(t, agent.DefaultAllowedTools, filled.AllowedTools)
	require.Equal(t, agent.DefaultTimeoutSeconds, filled.TimeoutSeconds)

	require.Equal(t, agent.MinTimeoutSeconds, (&agent.Config{TimeoutSeconds: 1}).Normalized().TimeoutSeconds)
	require.Equal(t, agent.MaxTimeoutSeconds, (&agent.Config{TimeoutSeconds: 999_999}).Normalized().TimeoutSeconds)
	require.Equal(t, agent.DefaultTimeoutSeconds, (*agent.Config)(nil).Normalized().TimeoutSeconds)
}

func TestLegalProvidersIsACopy(t *testing.T) {
	t.Parallel()

	providers := agent.LegalProviders()
	require.Equal(t, []string{agent.ProviderClaude, agent.ProviderCodex}, providers)

	providers[0] = "mutated"

	require.Equal(t, agent.ProviderClaude, agent.LegalProviders()[0])

	require.True(t, agent.IsLegalProvider(agent.ProviderClaude))
	require.False(t, agent.IsLegalProvider("gemini"))
}

// observed collects the notes of one run, guarded because the answer stream and
// the error stream are narrated from two goroutines.
type observed struct {
	mu     sync.Mutex
	levels []string
	texts  []string
}

func (o *observed) Note(level, text string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.levels = append(o.levels, level)
	o.texts = append(o.texts, text)
}

func (o *observed) document() string {
	o.mu.Lock()
	defer o.mu.Unlock()

	return strings.Join(o.texts, "\n")
}

func (o *observed) hasLevel(level string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	return slices.Contains(o.levels, level)
}

//nolint:gosmopolitan //the narration this asserts on is chinese by design.
func TestRunTellsTheObserverWhatHappened(t *testing.T) {
	t.Parallel()

	stream := `{"type":"system","subtype":"init","tools":["WebSearch"]}` + "\n" +
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"WebSearch","input":{"query":"go news"}}]}}` + "\n" +
		envelope(`{"items":[{"title":"a","url":"`+sampleURL+`"}]}`)

	stub := fakeClaude(t, stream, 0)
	watcher := &observed{}

	result, err := agent.Run(t.Context(), &agent.Config{Command: stub.binary}, testInstruction, 3, watcher)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)

	document := watcher.document()
	require.Contains(t, document, "启动 "+stub.binary)
	require.Contains(t, document, "提示词")
	require.Contains(t, document, testInstruction)
	require.Contains(t, document, "会话已启动")
	require.Contains(t, document, `调用 WebSearch {"query":"go news"}`)
	require.Contains(t, document, "原始返回")
}

func TestRunShowsTheObserverWhatTheAgentComplainedAbout(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == windowsGOOS {
		t.Skip("the fake agent is a posix shell script")
	}

	dir := t.TempDir()
	noisy := filepath.Join(dir, "noisy-claude")
	script := "#!/bin/sh\n" +
		"echo 'deprecation warning' >&2\n" +
		"echo '" + envelope(`{"items":[]}`) + "'\n"

	require.NoError(t, os.WriteFile(noisy, []byte(script), 0o700)) //nolint:gosec //test helper binary.

	watcher := &observed{}

	_, err := agent.Run(t.Context(), &agent.Config{Command: noisy}, testInstruction, 3, watcher)
	require.NoError(t, err)

	require.Contains(t, watcher.document(), "deprecation warning")
	require.True(t, watcher.hasLevel(agent.NoteWarn))
}
