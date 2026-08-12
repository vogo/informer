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
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/agent"
)

func TestDefaultCommandName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, agent.ClaudeCommand, agent.DefaultCommandName(agent.ProviderClaude))
	assert.Equal(t, agent.ClaudeCommand, agent.DefaultCommandName(""))
	assert.Equal(t, agent.CodexCommand, agent.DefaultCommandName(agent.ProviderCodex))
}

func TestResolveCommandUsesAnAbsoluteExecutable(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("the fixture is a posix shell script")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "my-claude")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec //test helper binary.

	resolved, err := agent.ResolveCommand(agent.ProviderClaude, binary)
	require.NoError(t, err)
	assert.Equal(t, binary, resolved)
}

func TestResolveCommandFindsABareNameOnPATH(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("the fixture is a posix shell script")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "path-claude")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec //test helper binary.

	t.Setenv("PATH", dir)

	resolved, err := agent.ResolveCommand(agent.ProviderClaude, "path-claude")
	require.NoError(t, err)
	assert.Equal(t, binary, resolved)
}

func TestResolveCommandFindsABareNameInACommonDir(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("the fixture is a posix shell script")
	}

	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o700))

	// a unique name avoids matching a real install under /opt/homebrew/bin.
	name := "informer-locate-test"
	binary := filepath.Join(binDir, name)
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700)) //nolint:gosec //test helper binary.

	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin")

	resolved, err := agent.ResolveCommand(agent.ProviderClaude, name)
	require.NoError(t, err)
	assert.Equal(t, binary, resolved)
}

func TestResolveCommandRejectsAMissingPath(t *testing.T) {
	t.Parallel()

	_, err := agent.ResolveCommand(agent.ProviderClaude, filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, agent.ErrCommandNotFound)
}
