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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
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
)

// claudeEnvelope is the --output-format json document the command line prints.
// Only the fields informer acts on are declared; everything else - cost, token
// usage, session id - is deliberately ignored.
type claudeEnvelope struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Error   string `json:"error"`
}

// runClaude executes one non interactive Claude Code run and returns its answer text.
func runClaude(ctx context.Context, cfg *Config, prompt string) (string, error) {
	command := cfg.Command
	if command == "" {
		command = ClaudeCommand
	}

	cmd := exec.CommandContext(ctx, command, claudeArgs(cfg, prompt)...)
	cmd.Dir = cfg.WorkDir
	cmd.Env = claudeEnv(cfg)
	cmd.Stdin = nil

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// the timeout is the common failure of a browsing agent, and the raw
		// exec message does not say so; name it where the source error is stored.
		ctxErr := ctx.Err()
		if ctxErr != nil {
			return "", fmt.Errorf("%w: %s run stopped after %ds: %w",
				ErrAgentFailed, command, cfg.TimeoutSeconds, ctxErr)
		}

		// a silent failure - a missing binary, a killed process - carries no
		// stderr, and appending an empty tail would only end the stored source
		// error in a dangling colon.
		if details := truncate(stderr.String()); details != "" {
			return "", fmt.Errorf("%w: %s run: %w: %s", ErrAgentFailed, command, err, details)
		}

		return "", fmt.Errorf("%w: %s run: %w", ErrAgentFailed, command, err)
	}

	return claudeResultText(stdout.Bytes())
}

// claudeArgs builds the non interactive invocation.
//
// The tool set is passed twice on purpose: --tools bounds what the session may
// reach for at all, and --allowedTools pre-approves exactly that set so a
// headless run never stalls on a permission prompt it has no way to answer.
func claudeArgs(cfg *Config, prompt string) []string {
	args := []string{"--print", prompt, "--output-format", "json", "--strict-mcp-config"}

	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}

	if cfg.AllowedTools != "" {
		args = append(args, "--tools", cfg.AllowedTools, "--allowedTools", cfg.AllowedTools)
	}

	return args
}

// claudeEnv builds the child environment: the current one, with the configured
// endpoint and credential replacing whatever it already carried. An unconfigured
// field is left alone rather than blanked, which is what makes "just run claude
// with the machine's own login" work.
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

// claudeResultText unwraps the json envelope and returns the answer text.
func claudeResultText(output []byte) (string, error) {
	var envelope claudeEnvelope

	err := json.Unmarshal(bytes.TrimSpace(output), &envelope)
	if err != nil {
		return "", fmt.Errorf("parse claude output envelope: %w: %s", err, truncate(string(output)))
	}

	if envelope.IsError {
		reason := envelope.Error
		if reason == "" {
			reason = envelope.Result
		}

		return "", fmt.Errorf("%w: claude reported %q: %s", ErrAgentFailed, envelope.Subtype, truncate(reason))
	}

	return envelope.Result, nil
}
