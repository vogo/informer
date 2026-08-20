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

package diagnose

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vogo/informer/internal/feed"
)

// SessionFileName is the document one diagnosis run is described by. It lives in
// a directory of its own that the parent process creates and removes, and the
// mcp child process reads it instead of opening the database.
//
// Keeping the child away from sqlite is the point: the window holds the same
// file open for the whole session, and a second writer would contend for the
// lock only on the machines where a diagnosis takes minutes - the last place a
// failure should first appear.
const SessionFileName = "session.json"

// MCPConfigFileName is the mcp server document handed to the agent command line.
const MCPConfigFileName = "mcp.json"

// ServerName is how the mcp server is addressed. It is part of the tool names
// the agent sees - mcp__informer__try_parse - so it is short and stable.
const ServerName = "informer"

// ErrNoSession marks a directory that carries no readable session document.
var ErrNoSession = errors.New("diagnose session not found")

// Session is everything one diagnosis run works on.
//
// The source is a snapshot taken when the run started, not a live record: the
// child process must diagnose the configuration that failed, even if the window
// edits the stored one while the agent is thinking.
type Session struct {
	// SourceID identifies the stored subscription the diagnosis is about.
	SourceID int64 `json:"source_id"`

	// Source is the stored configuration as it was when the run started.
	Source *feed.Source `json:"source"`

	// StoredError is the failure recorded on the source by its last real fetch,
	// which may be older and more informative than a fresh attempt.
	StoredError string `json:"stored_error"`

	// FreshError is what a parse attempted at the start of this run produced.
	// It is empty when that attempt unexpectedly succeeded, which is itself
	// worth telling the agent: an intermittent source is not a broken one.
	FreshError string `json:"fresh_error"`

	// HTTPProxy is the proxy the parent uses for its own fetches, so the child's
	// fetches go out the same way. An empty value means direct.
	HTTPProxy string `json:"http_proxy"`
}

// WriteSession stores the session document inside dir, creating the directory.
func WriteSession(dir string, session *Session) error {
	if session == nil {
		return fmt.Errorf("%w: session is nil", ErrNoSession)
	}

	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return fmt.Errorf("create diagnose dir: %w", err)
	}

	encoded, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("encode diagnose session: %w", err)
	}

	// the snapshot carries whatever a curl line carries, credentials included,
	// so it is written as tightly as the credential file is.
	err = os.WriteFile(filepath.Join(dir, SessionFileName), encoded, 0o600)
	if err != nil {
		return fmt.Errorf("write diagnose session: %w", err)
	}

	return nil
}

// ReadSession loads the session document of a run directory.
func ReadSession(dir string) (*Session, error) {
	data, err := os.ReadFile(filepath.Join(dir, SessionFileName))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoSession, err)
	}

	var session Session

	err = json.Unmarshal(data, &session)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoSession, err)
	}

	if session.Source == nil {
		return nil, fmt.Errorf("%w: session carries no source", ErrNoSession)
	}

	return &session, nil
}

// MCPServerConfig is the document the agent command line loads to reach this
// server: one stdio server, launched from the given executable.
type MCPServerConfig struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
}

// MCPServerEntry describes one stdio server.
type MCPServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// WriteMCPConfig stores the mcp server document of a run and returns its path.
//
// command is the executable the agent should launch to reach the tools - the
// running informer binary itself - and args are what puts that binary into
// server mode for this run directory.
func WriteMCPConfig(dir, command string, args ...string) (string, error) {
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		return "", fmt.Errorf("create diagnose dir: %w", err)
	}

	encoded, err := json.MarshalIndent(MCPServerConfig{
		MCPServers: map[string]MCPServerEntry{
			ServerName: {Command: command, Args: args},
		},
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode mcp config: %w", err)
	}

	path := filepath.Join(dir, MCPConfigFileName)

	err = os.WriteFile(path, encoded, 0o600)
	if err != nil {
		return "", fmt.Errorf("write mcp config: %w", err)
	}

	return path, nil
}

// AllowedTools is the tool set a diagnosis run may use, in the form the agent
// command line expects.
//
// Only the three server tools are listed. A diagnosis that could search the web
// would spend its time reading documentation about the site instead of the bytes
// informer received, and reading those bytes is the entire job.
func AllowedTools() string {
	names := make([]string, 0, len(toolNames))
	for _, name := range toolNames {
		names = append(names, "mcp__"+ServerName+"__"+name)
	}

	return strings.Join(names, ",")
}
