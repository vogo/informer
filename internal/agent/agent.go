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

// Package agent turns a plain language instruction into a list of articles by
// driving a coding agent command line.
//
// The caller only supplies the instruction: this package appends the machine
// readable output contract, runs the agent, and parses what came back. That split
// is the whole point - a source author writes what they want in their own words
// and never has to know the json shape informer expects.
package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Supported agent providers.
const (
	// ProviderClaude drives the "claude" command line of Claude Code.
	ProviderClaude = "claude"

	// ProviderCodex names the codex command line. It is accepted by the
	// configuration and refused at run time until a runner exists, so a source
	// can already record the intent without silently running the wrong agent.
	ProviderCodex = "codex"
)

// Errors returned by this package.
var (
	// ErrUnsupportedProvider marks a provider this build cannot run.
	ErrUnsupportedProvider = errors.New("unsupported agent provider")

	// ErrEmptyPrompt marks a run without any instruction to give the agent.
	ErrEmptyPrompt = errors.New("agent prompt is empty")

	// ErrNoJSONOutput marks output that carries no json document at all.
	ErrNoJSONOutput = errors.New("agent output carries no json result")

	// ErrAgentFailed marks a run the agent itself reported as failed.
	ErrAgentFailed = errors.New("agent run failed")
)

// Defaults and bounds of one agent run.
const (
	// DefaultProvider is the agent used when a configuration names none.
	DefaultProvider = ProviderClaude

	// DefaultTimeoutSeconds bounds one agent run. An agent that browses the web
	// is far slower than an http fetch, so the window is minutes, not seconds.
	DefaultTimeoutSeconds = 300

	// MinTimeoutSeconds and MaxTimeoutSeconds bound the configured timeout.
	MinTimeoutSeconds = 10
	MaxTimeoutSeconds = 3600

	// DefaultAllowedTools is the tool set an agent source may use. Reading the
	// public web is what a subscription needs; nothing here can write.
	DefaultAllowedTools = "WebSearch,WebFetch"

	// DefaultMaxItems bounds how many articles one run is asked for when the
	// source sets no fetch limit of its own.
	DefaultMaxItems = 20

	// MaxItemsCap is the highest item count a prompt ever asks for.
	MaxItemsCap = 100
)

// Config is everything one agent run needs beyond the instruction itself.
//
// The json tags describe the "agent" section of informer.json. APIKey carries no
// tag on purpose: a credential belongs in the separate secret file, never in the
// shareable configuration document.
type Config struct {
	// Provider selects the agent command line, one of the Provider constants.
	Provider string `json:"provider"`

	// BaseURL overrides the agent api endpoint. An empty value leaves whatever
	// the machine is already configured with, which is how "just run claude"
	// with the user's own login keeps working.
	BaseURL string `json:"base_url"`

	// APIKey authenticates against BaseURL. An empty value leaves the agent's
	// own credentials in place.
	APIKey string `json:"-"`

	// Model overrides the model the agent runs. An empty value keeps its default.
	Model string `json:"model"`

	// AllowedTools is the comma separated tool set the agent may use.
	AllowedTools string `json:"allowed_tools"`

	// TimeoutSeconds bounds one run; the run is killed when it is exceeded.
	TimeoutSeconds int `json:"timeout_seconds"`

	// Command overrides the executable path. An empty value is resolved to the
	// provider default by searching PATH, common install directories and the
	// user's login shell, then remembered in informer.json on the first hit.
	Command string `json:"command"`

	// WorkDir is the directory the agent process runs in. It is set by the
	// caller rather than configured, so it never lands in informer.json.
	WorkDir string `json:"-"`

	// HTTPProxy is the HTTP(S) proxy the agent process should use. It is filled
	// from the top level http_proxy of informer.json at run time, never stored
	// inside the agent section.
	HTTPProxy string `json:"-"`
}

// DefaultConfig is the agent section a data directory without one starts from.
func DefaultConfig() *Config {
	return &Config{
		Provider:       DefaultProvider,
		AllowedTools:   DefaultAllowedTools,
		TimeoutSeconds: DefaultTimeoutSeconds,
	}
}

// legalProviders is the ordered set of providers the configuration accepts.
// Whether a provider can actually run is a separate question answered by runnerOf.
var legalProviders = []string{ProviderClaude, ProviderCodex} //nolint:gochecknoglobals //ordered constant set.

// LegalProviders returns the accepted providers in presentation order.
// The result is a copy, so a caller can neither shrink nor reorder the set.
func LegalProviders() []string {
	return slices.Clone(legalProviders)
}

// IsLegalProvider reports whether the value names a provider the configuration accepts.
func IsLegalProvider(provider string) bool {
	return slices.Contains(legalProviders, provider)
}

// Item is one article an agent returned.
type Item struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Result is the outcome of one agent run.
type Result struct {
	// Items are the articles parsed out of the agent output.
	Items []Item

	// Raw is the agent's own answer text, kept so a source that returned
	// nothing usable can still be diagnosed from the logs.
	Raw string
}

// Normalized returns a copy with every empty field replaced by its default and
// every out of range value clamped, so a runner never has to defend itself
// against a half filled configuration.
func (c *Config) Normalized() *Config {
	normalized := &Config{}
	if c != nil {
		*normalized = *c
	}

	normalized.Provider = strings.TrimSpace(normalized.Provider)
	if normalized.Provider == "" {
		normalized.Provider = DefaultProvider
	}

	normalized.BaseURL = strings.TrimSpace(normalized.BaseURL)
	normalized.APIKey = strings.TrimSpace(normalized.APIKey)
	normalized.Model = strings.TrimSpace(normalized.Model)
	normalized.Command = strings.TrimSpace(normalized.Command)
	normalized.HTTPProxy = strings.TrimSpace(normalized.HTTPProxy)

	normalized.AllowedTools = strings.TrimSpace(normalized.AllowedTools)
	if normalized.AllowedTools == "" {
		normalized.AllowedTools = DefaultAllowedTools
	}

	switch {
	case normalized.TimeoutSeconds <= 0:
		normalized.TimeoutSeconds = DefaultTimeoutSeconds
	case normalized.TimeoutSeconds < MinTimeoutSeconds:
		normalized.TimeoutSeconds = MinTimeoutSeconds
	case normalized.TimeoutSeconds > MaxTimeoutSeconds:
		normalized.TimeoutSeconds = MaxTimeoutSeconds
	}

	return normalized
}

// runner executes one instruction and returns the agent's answer text, telling
// observer what it is doing while it does it.
type runner func(ctx context.Context, cfg *Config, prompt string, observer Observer) (string, error)

// runnerOf returns the runner of a provider, or an error for one this build
// accepts in configuration but cannot execute yet.
func runnerOf(provider string) (runner, error) {
	if provider == ProviderClaude {
		return runClaude, nil
	}

	if provider == ProviderCodex {
		return nil, fmt.Errorf("%w: %q is not implemented in this build", ErrUnsupportedProvider, provider)
	}

	return nil, fmt.Errorf("%w: %q", ErrUnsupportedProvider, provider)
}

// Run gives the agent the user's instruction plus the appended output contract,
// and returns the articles its answer described.
//
// maxItems bounds what the prompt asks for; a value of zero uses DefaultMaxItems.
// The run is always bounded by the configured timeout, whatever deadline ctx carries.
//
// observer, when not nil, is told what the run is doing as it happens: the whole
// prompt that went out, every search and page the agent reached for, and the
// answer that came back. A run watched this way is one a user can debug.
//
//nolint:gosmopolitan //the notes are read by a chinese user in a chinese window.
func Run(ctx context.Context, cfg *Config, userPrompt string, maxItems int, observer Observer) (*Result, error) {
	instruction := strings.TrimSpace(userPrompt)
	if instruction == "" {
		return nil, ErrEmptyPrompt
	}

	config := cfg.Normalized()

	run, err := runnerOf(config.Provider)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()

	prompt := BuildPrompt(instruction, maxItems)

	notef(observer, NoteInfo, "提示词（%d 字符）：\n%s", len([]rune(prompt)), truncateTo(prompt, maxNoteRunes))

	raw, err := run(runCtx, config, prompt, observer)
	if err != nil {
		return nil, err
	}

	notef(observer, NoteInfo, "原始返回（%d 字符）：\n%s", len([]rune(raw)), truncateTo(raw, maxNoteRunes))

	items, err := ParseItems(raw)
	if err != nil {
		return &Result{Raw: raw}, err
	}

	return &Result{Items: items, Raw: raw}, nil
}
