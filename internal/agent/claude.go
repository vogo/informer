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

package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// ClaudeCommand is the default executable of the Claude Code command line.
const ClaudeCommand = "claude"

// Environment variables the claude command line reads. They are set only when the
// configuration fills them in, so a machine already logged in through `claude`
// itself keeps running with its own credentials.
const (
	envBaseURL   = "ANTHROPIC_BASE_URL"
	envAuthToken = "ANTHROPIC_AUTH_TOKEN" //nolint:gosec //variable name, not a credential.
	envAPIKey    = "ANTHROPIC_API_KEY"    //nolint:gosec //variable name, not a credential.

	envHTTPProxy  = "HTTP_PROXY"
	envHTTPSProxy = "HTTPS_PROXY"
	envAllProxy   = "ALL_PROXY"
)

// runClaude executes one non interactive Claude Code run and returns its answer text.
//
// The run is read as it happens rather than waited out: a browsing agent spends
// minutes searching, and an observer that sees "searching for X" as it goes is
// the difference between a diagnosable source and a spinner.
//
//nolint:gosmopolitan //informer is a chinese product; the notes speak the user's language.
func runClaude(ctx context.Context, cfg *Config, prompt string, observer Observer) (string, error) {
	command, err := ResolveCommand(cfg.Provider, cfg.Command)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrAgentFailed, err)
	}

	cmd := exec.CommandContext(ctx, command, claudeArgs(cfg, prompt)...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = claudeEnv(cfg)
	cmd.Stdin = nil

	answers, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("%w: %s stdout: %w", ErrAgentFailed, command, err)
	}

	complaints, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("%w: %s stderr: %w", ErrAgentFailed, command, err)
	}

	notef(observer, NoteInfo, "启动 %s，超时上限 %ds", command, cfg.TimeoutSeconds)

	err = cmd.Start()
	if err != nil {
		return "", fmt.Errorf("%w: %s start: %w", ErrAgentFailed, command, err)
	}

	var (
		stderr strings.Builder
		group  sync.WaitGroup
	)

	group.Go(func() {
		drainStderr(complaints, &stderr, observer)
	})

	result, streamErr := readClaudeStream(answers, observer)

	group.Wait()

	err = cmd.Wait()
	if err != nil {
		return "", claudeRunError(ctx, cfg, command, err, stderr.String())
	}

	if streamErr != nil {
		return "", streamErr
	}

	return result, nil
}

// claudeRunError names why the process ended badly, keeping the timeout - the
// common failure of a browsing agent - distinguishable from a crash.
func claudeRunError(ctx context.Context, cfg *Config, command string, runErr error, stderr string) error {
	// the raw exec message does not mention the deadline; name it where the
	// stored source error will be read.
	ctxErr := ctx.Err()
	if ctxErr != nil {
		return fmt.Errorf("%w: %s run stopped after %ds: %w",
			ErrAgentFailed, command, cfg.TimeoutSeconds, ctxErr)
	}

	// a silent failure - a missing binary, a killed process - carries no
	// stderr, and appending an empty tail would only end the stored source
	// error in a dangling colon.
	if details := truncate(stderr); details != "" {
		return fmt.Errorf("%w: %s run: %w: %s", ErrAgentFailed, command, runErr, details)
	}

	return fmt.Errorf("%w: %s run: %w", ErrAgentFailed, command, runErr)
}

// drainStderr keeps the error stream flowing - a full pipe would deadlock the
// child - while showing each line to the observer and keeping a copy for the
// error message.
func drainStderr(reader io.Reader, kept *strings.Builder, observer Observer) {
	for line := range lines(reader) {
		kept.WriteString(line)
		kept.WriteString("\n")

		notef(observer, NoteWarn, "%s", truncateTo(line, maxNoteRunes))
	}
}

// claudeArgs builds the non interactive invocation.
//
// The output format is the streaming one so the run can be watched; --verbose is
// what the command line requires to actually emit the intermediate events under
// --print, and without it the stream carries the final result alone.
//
// The tool set is passed twice on purpose: --tools bounds what the session may
// reach for at all, and --allowedTools pre-approves exactly that set so a
// headless run never stalls on a permission prompt it has no way to answer.
func claudeArgs(cfg *Config, prompt string) []string {
	args := []string{
		"--print", prompt,
		"--output-format", "stream-json",
		"--verbose",
		"--strict-mcp-config",
	}

	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}

	if cfg.AllowedTools != "" {
		args = append(args, "--tools", cfg.AllowedTools, "--allowedTools", cfg.AllowedTools)
	}

	return args
}

// claudeEnv builds the child environment: the current one, with the configured
// endpoint, credential and proxy replacing whatever it already carried. An
// unconfigured field is left alone rather than blanked, which is what makes
// "just run claude with the machine's own login" work.
func claudeEnv(cfg *Config) []string {
	overrides := map[string]string{}

	if cfg.BaseURL != "" {
		overrides[envBaseURL] = cfg.BaseURL
	}

	if cfg.APIKey != "" {
		// both forms are set because an Anthropic compatible endpoint may read
		// either the bearer token or the x-api-key header.
		overrides[envAuthToken] = cfg.APIKey
		overrides[envAPIKey] = cfg.APIKey
	}

	if cfg.HTTPProxy != "" {
		// all three names are set so http, https and socks-aware clients inside
		// the agent process honour the same configured proxy.
		overrides[envHTTPProxy] = cfg.HTTPProxy
		overrides[envHTTPSProxy] = cfg.HTTPProxy
		overrides[envAllProxy] = cfg.HTTPProxy
	}

	if len(overrides) == 0 {
		return nil
	}

	env := os.Environ()
	kept := make([]string, 0, len(env)+len(overrides))

	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := overrides[name]; replaced {
				continue
			}
		}

		kept = append(kept, entry)
	}

	for name, value := range overrides {
		kept = append(kept, name+"="+value)
	}

	return kept
}

// errNoResultEvent marks a stream that ended without the closing result event.
var errNoResultEvent = errors.New("printed no result event")

// readClaudeStream consumes the newline delimited json the command line prints,
// narrating it to the observer, and returns the answer text of the closing
// result event.
func readClaudeStream(reader io.Reader, observer Observer) (string, error) {
	var (
		answer  string
		settled bool
		failure error
		tail    []string
	)

	for line := range lines(reader) {
		tail = keepTail(tail, line)

		event, ok := decodeClaudeEvent(line)
		if !ok {
			continue
		}

		if event.Type == eventResult {
			answer, failure = claudeResultText(event)
			settled = true

			continue
		}

		narrate(event, observer)
	}

	if failure != nil {
		return "", failure
	}

	if !settled {
		return "", fmt.Errorf("%w: %s %w: %s",
			ErrAgentFailed, ProviderClaude, errNoResultEvent, truncate(strings.Join(tail, "\n")))
	}

	return answer, nil
}

// maxTailLines is how much of an unusable stream is quoted back in the error.
const maxTailLines = 5

// keepTail remembers the last lines of a stream for the diagnostic message.
func keepTail(tail []string, line string) []string {
	tail = append(tail, line)
	if len(tail) > maxTailLines {
		tail = tail[len(tail)-maxTailLines:]
	}

	return tail
}

// lines yields the non empty lines of a reader.
//
// It reads with a plain reader rather than a bufio.Scanner because one event of
// an agent stream - a fetched page handed back as a tool result - easily passes
// the scanner's 64KB line limit, and a run that dies on a large answer is worse
// than no streaming at all.
func lines(reader io.Reader) func(func(string) bool) {
	return func(yield func(string) bool) {
		buffered := bufio.NewReader(reader)

		for {
			line, err := buffered.ReadString('\n')

			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !yield(trimmed) {
				return
			}

			if err != nil {
				return
			}
		}
	}
}
