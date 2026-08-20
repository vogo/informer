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
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
)

// uuidV4 is the shape the command line insists a session name has.
var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// turnRecorder is a stand-in command line that remembers each turn separately.
//
// The single record file of fakeClaude cannot answer the questions a
// conversation raises - what the second invocation looked like, whether the
// third reused the first's session name - so this one numbers its turns and
// keeps an ordered log of when each started and finished.
type turnRecorder struct {
	binary string
	dir    string
}

// failTurn makes the given turn number exit non zero.
func (r turnRecorder) failTurn(t *testing.T, turn int) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(r.dir, "fail-"+strconv.Itoa(turn)), nil, 0o600))
}

// args returns the arguments the given turn was invoked with.
func (r turnRecorder) args(t *testing.T, turn int) []string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(r.dir, "turn-"+strconv.Itoa(turn)))
	require.NoError(t, err)

	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// order returns the start and end markers of every turn, in the order they were
// written. Interleaved markers are what a session that did not serialize its
// turns would leave behind.
func (r turnRecorder) order(t *testing.T) []string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(r.dir, "order"))
	require.NoError(t, err)

	return strings.Fields(string(data))
}

// value returns the argument following the named flag, or an empty string when
// the flag is absent.
func value(args []string, flag string) string {
	at := slicesIndex(args, flag)
	if at < 0 || at+1 >= len(args) {
		return ""
	}

	return args[at+1]
}

func slicesIndex(args []string, want string) int {
	for i, arg := range args {
		if arg == want {
			return i
		}
	}

	return -1
}

// fakeSessionClaude writes a stand-in that records every turn it is invoked for.
func fakeSessionClaude(t *testing.T, answer string, delay string) turnRecorder {
	t.Helper()

	if runtime.GOOS == windowsGOOS {
		t.Skip("the fake agent is a posix shell script")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-claude")

	script := "#!/bin/sh\n" +
		"n=$(cat " + dir + "/counter 2>/dev/null || echo 0)\n" +
		"n=$((n+1))\n" +
		"echo $n > " + dir + "/counter\n" +
		"printf 's%s ' \"$n\" >> " + dir + "/order\n" +
		"{ for arg in \"$@\"; do printf '%s\\n' \"$arg\"; done; } > " + dir + "/turn-$n\n" +
		"sleep " + delay + "\n" +
		"printf 'e%s ' \"$n\" >> " + dir + "/order\n" +
		"if [ -f " + dir + "/fail-$n ]; then exit 1; fi\n" +
		"cat <<'INFORMER_EOF'\n" + answer + "\nINFORMER_EOF\n"

	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700)) //nolint:gosec //test helper binary.

	return turnRecorder{binary: binary, dir: dir}
}

// The first turn names the conversation and every later one continues it. The
// command line refuses an invocation that does both, so this is not a style
// choice: getting it wrong turns every turn into a fresh conversation, or into
// an error, depending on which flag is dropped.
func TestSessionNamesThenResumesOneConversation(t *testing.T) {
	t.Parallel()

	stub := fakeSessionClaude(t, envelope("ok"), "0")
	session := agent.NewSession(&agent.Config{Command: stub.binary})

	_, err := session.Send(t.Context(), "first", nil)
	require.NoError(t, err)

	_, err = session.Send(t.Context(), "second", nil)
	require.NoError(t, err)

	first := stub.args(t, 1)
	second := stub.args(t, 2)

	named := value(first, "--session-id")
	require.Regexp(t, uuidV4, named)
	require.NotContains(t, first, "--resume")

	require.Equal(t, named, value(second, "--resume"))
	require.NotContains(t, second, "--session-id")
	require.Equal(t, 2, session.Turns())
	require.Equal(t, named, session.ID())
}

// Command line arguments are not part of the transcript, so a resumed turn that
// stopped passing them would be a turn whose tools and rules silently vanished.
func TestSessionRepeatsTheInvocationOnEveryTurn(t *testing.T) {
	t.Parallel()

	stub := fakeSessionClaude(t, envelope("ok"), "0")
	session := agent.NewSession(&agent.Config{
		Command:            stub.binary,
		MCPConfigPath:      "/tmp/mcp.json",
		AllowedTools:       "mcp__informer__try_parse",
		AppendSystemPrompt: "the rules",
		Model:              "some-model",
	})

	for range 2 {
		_, err := session.Send(t.Context(), "hello", nil)
		require.NoError(t, err)
	}

	for turn := 1; turn <= 2; turn++ {
		args := stub.args(t, turn)

		require.Contains(t, args, "--strict-mcp-config")
		require.Equal(t, "/tmp/mcp.json", value(args, "--mcp-config"))
		require.Equal(t, "mcp__informer__try_parse", value(args, "--tools"))
		require.Equal(t, "mcp__informer__try_parse", value(args, "--allowedTools"))
		require.Equal(t, "the rules", value(args, "--append-system-prompt"))
		require.Equal(t, "some-model", value(args, "--model"))
	}
}

// A turn that died leaves a session name that cannot be claimed again and a
// transcript that is not worth continuing. Starting over loses the history,
// which is the honest outcome; resuming it would fail on every later turn.
func TestSessionStartsOverAfterAFailedTurn(t *testing.T) {
	t.Parallel()

	stub := fakeSessionClaude(t, envelope("ok"), "0")
	stub.failTurn(t, 1)

	session := agent.NewSession(&agent.Config{Command: stub.binary})

	_, err := session.Send(t.Context(), "first", nil)
	require.Error(t, err)
	require.Empty(t, session.ID())
	require.Equal(t, 0, session.Turns())

	_, err = session.Send(t.Context(), "second", nil)
	require.NoError(t, err)

	first := value(stub.args(t, 1), "--session-id")
	second := value(stub.args(t, 2), "--session-id")

	require.Regexp(t, uuidV4, second)
	require.NotEqual(t, first, second)
	require.NotContains(t, stub.args(t, 2), "--resume")
}

// Two turns of one conversation in flight at once would have the command line
// resume a transcript that is still being written.
func TestSessionSerializesConcurrentTurns(t *testing.T) {
	t.Parallel()

	stub := fakeSessionClaude(t, envelope("ok"), "0.2")
	session := agent.NewSession(&agent.Config{Command: stub.binary})

	var wait sync.WaitGroup

	for range 2 {
		wait.Add(1)

		go func() {
			defer wait.Done()

			_, err := session.Send(t.Context(), "hello", nil)
			require.NoError(t, err) //nolint:testifylint //a goroutine cannot stop the test.
		}()
	}

	wait.Wait()

	require.Equal(t, []string{"s1", "e1", "s2", "e2"}, stub.order(t))
}

// A one shot run names no conversation at all: a fetching source and a diagnosis
// both want the process to forget them the moment it exits.
func TestRunRawNamesNoSession(t *testing.T) {
	t.Parallel()

	stub := fakeSessionClaude(t, envelope("ok"), "0")

	_, err := agent.RunRaw(t.Context(), &agent.Config{Command: stub.binary}, "hello", nil)
	require.NoError(t, err)

	args := stub.args(t, 1)
	require.NotContains(t, args, "--session-id")
	require.NotContains(t, args, "--resume")
	require.NotContains(t, args, "--append-system-prompt")
}
