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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/service"
)

// Values shared by the agent configuration cases.
const (
	testAgentBaseURL = "https://proxy.example.com"
	testAgentModel   = "claude-sonnet-5"
	testAgentAPIKey  = "secret-key"
	unknownAgentName = "gemini"
)

func TestReadAgentConfigFallsBackToTheDefaults(t *testing.T) {
	svc := newService(t)

	stored, err := svc.ReadAgentConfig()
	require.NoError(t, err)
	assert.Equal(t, service.DefaultAgentConfig(), stored)
}

func TestSaveAgentConfigRoundTrips(t *testing.T) {
	svc := newService(t)

	saved := &agent.Config{
		Provider:       agent.ProviderClaude,
		BaseURL:        testAgentBaseURL,
		Model:          testAgentModel,
		AllowedTools:   "WebFetch",
		TimeoutSeconds: 120,
		Command:        "/opt/bin/claude",
	}
	require.NoError(t, svc.SaveAgentConfig(saved))

	stored, err := svc.ReadAgentConfig()
	require.NoError(t, err)
	assert.Equal(t, saved, stored)

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	require.NotNil(t, view.Agent)
	assert.Equal(t, testAgentBaseURL, view.Agent.BaseURL)

	// the agent section is edited by this build, so it is never listed as a
	// field that merely survives the save.
	assert.NotContains(t, view.PreservedKeys, "agent")
}

func TestSaveAgentConfigKeepsTheOtherSections(t *testing.T) {
	svc := newService(t)

	require.NoError(t, svc.SaveAgentConfig(&agent.Config{Model: testAgentModel}))

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	require.NotNil(t, view.Feed, "the feed section written by newService survives an agent save")
	assert.Equal(t, 10, view.Feed.MaxInformFeedSize)
}

func TestSaveAgentConfigRejectsAnUnusableSection(t *testing.T) {
	svc := newService(t)

	require.ErrorIs(t, svc.SaveAgentConfig(nil), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.SaveAgentConfig(&agent.Config{Provider: unknownAgentName}), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.SaveAgentConfig(&agent.Config{TimeoutSeconds: 1}), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.SaveAgentConfig(&agent.Config{TimeoutSeconds: 99_999}), service.ErrInvalidArgument)

	// zero is the documented "use the default window" value, not an error.
	require.NoError(t, svc.SaveAgentConfig(&agent.Config{TimeoutSeconds: 0}))
}

func TestAgentAPIKeyIsStoredInTheCredentialFile(t *testing.T) {
	svc := newService(t)

	view, err := svc.ReadSecretsView()
	require.NoError(t, err)
	assert.False(t, view.AgentAPIKeyConfigured)

	require.NoError(t, svc.SaveAgentAPIKey("  "+testAgentAPIKey+"  "))

	view, err = svc.ReadSecretsView()
	require.NoError(t, err)
	assert.True(t, view.AgentAPIKeyConfigured)

	// the key never leaves the service through the view: the page is told that
	// one exists, not what it is.
	raw, err := os.ReadFile(svc.SecretsFilePath())
	require.NoError(t, err)
	assert.Contains(t, string(raw), testAgentAPIKey)
	assert.NotContains(t, string(raw), "  "+testAgentAPIKey+"  ")

	if runtime.GOOS != windowsGOOS {
		info, statErr := os.Stat(svc.SecretsFilePath())
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	// clearing puts the installation back on the agent's own login.
	require.NoError(t, svc.SaveAgentAPIKey(""))

	view, err = svc.ReadSecretsView()
	require.NoError(t, err)
	assert.False(t, view.AgentAPIKeyConfigured)
}

func TestAgentAPIKeyAndWebhookShareTheFileWithoutOverwriting(t *testing.T) {
	svc := newService(t)

	require.NoError(t, svc.SaveWebhook("https://open.feishu.cn/hook/x"))
	require.NoError(t, svc.SaveAgentAPIKey(testAgentAPIKey))

	view, err := svc.ReadSecretsView()
	require.NoError(t, err)
	assert.Equal(t, "https://open.feishu.cn/hook/x", view.Webhook)
	assert.True(t, view.AgentAPIKeyConfigured)

	require.NoError(t, svc.SaveWebhook(""))

	view, err = svc.ReadSecretsView()
	require.NoError(t, err)
	assert.False(t, view.WebhookConfigured)
	assert.True(t, view.AgentAPIKeyConfigured, "clearing one credential leaves the other in place")
}

func TestEffectiveAgentConfigCombinesFileSectionKeyAndHomeDir(t *testing.T) {
	svc := newService(t)

	require.NoError(t, svc.SaveAgentAPIKey(testAgentAPIKey))

	resolved := svc.EffectiveAgentConfig(&inform.Config{Agent: &agent.Config{
		BaseURL: testAgentBaseURL,
		Model:   testAgentModel,
	}})

	assert.Equal(t, testAgentBaseURL, resolved.BaseURL)
	assert.Equal(t, testAgentModel, resolved.Model)
	assert.Equal(t, testAgentAPIKey, resolved.APIKey)
	assert.Equal(t, svc.HomeDir(), resolved.WorkDir)

	// the unfilled fields come back normalized, so a runner never has to defend
	// itself against a half filled section.
	assert.Equal(t, agent.DefaultProvider, resolved.Provider)
	assert.Equal(t, agent.DefaultAllowedTools, resolved.AllowedTools)
	assert.Equal(t, agent.DefaultTimeoutSeconds, resolved.TimeoutSeconds)
}

func TestEffectiveAgentConfigWithoutASection(t *testing.T) {
	svc := newService(t)

	resolved := svc.EffectiveAgentConfig(nil)

	assert.Equal(t, agent.DefaultProvider, resolved.Provider)
	assert.Empty(t, resolved.BaseURL)
	assert.Empty(t, resolved.APIKey, "an installation without a stored key runs on the machine's own login")
	assert.Equal(t, filepath.Clean(svc.HomeDir()), filepath.Clean(resolved.WorkDir))
}

func TestEffectiveAgentConfigRemembersADiscoveredCommand(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("the fixture is a posix shell script")
	}

	svc := newService(t)

	path, err := agent.LocateCommand(agent.ProviderClaude)
	if err != nil {
		dir := t.TempDir()
		path = filepath.Join(dir, agent.ClaudeCommand)
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec //test helper binary.
		t.Setenv("PATH", dir)

		path, err = agent.LocateCommand(agent.ProviderClaude)
		require.NoError(t, err)
	}

	resolved := svc.EffectiveAgentConfig(nil)
	assert.Equal(t, path, resolved.Command)

	stored, err := svc.ReadAgentConfig()
	require.NoError(t, err)
	assert.Equal(t, path, stored.Command, "the discovered path is written back for the next run")
}

func TestEffectiveAgentConfigKeepsAnExplicitCommand(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("the fixture is a posix shell script")
	}

	svc := newService(t)

	dir := t.TempDir()
	binary := filepath.Join(dir, "custom-claude")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec //test helper binary.
	require.NoError(t, svc.SaveAgentConfig(&agent.Config{Command: binary}))

	other := filepath.Join(dir, agent.ClaudeCommand)
	require.NoError(t, os.WriteFile(other, []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec //test helper binary.
	t.Setenv("PATH", dir)

	resolved := svc.EffectiveAgentConfig(&inform.Config{Agent: &agent.Config{Command: binary}})
	assert.Equal(t, binary, resolved.Command)

	stored, err := svc.ReadAgentConfig()
	require.NoError(t, err)
	assert.Equal(t, binary, stored.Command, "an explicit command is never replaced by auto-detect")
}

func TestDetectAgentCommand(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("the fixture is a posix shell script")
	}

	svc := newService(t)

	dir := t.TempDir()
	binary := filepath.Join(dir, agent.ClaudeCommand)
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec //test helper binary.
	t.Setenv("PATH", dir)

	path, err := svc.DetectAgentCommand(agent.ProviderClaude)
	require.NoError(t, err)
	assert.Equal(t, binary, path)

	stored, err := svc.ReadAgentConfig()
	require.NoError(t, err)
	assert.Empty(t, stored.Command, "detect only reports the path; saving is left to the caller")

	_, err = svc.DetectAgentCommand(unknownAgentName)
	require.ErrorIs(t, err, service.ErrInvalidArgument)
}
