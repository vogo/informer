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

package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ServerConfig is the document an agent command line loads to reach a server.
type ServerConfig struct {
	MCPServers map[string]ServerEntry `json:"mcpServers"`
}

// ServerEntry describes one stdio server: what to launch and with what.
type ServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// WriteConfig stores a one server configuration document at path.
//
// The file is written as tightly as a credential file: the arguments name a run
// directory whose session document carries whatever a curl line carries.
func WriteConfig(path, name, command string, args ...string) error {
	encoded, err := json.MarshalIndent(ServerConfig{
		MCPServers: map[string]ServerEntry{
			name: {Command: command, Args: args},
		},
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode mcp config: %w", err)
	}

	err = os.WriteFile(path, encoded, 0o600)
	if err != nil {
		return fmt.Errorf("write mcp config: %w", err)
	}

	return nil
}

// QualifiedNames renders the tool names of a server the way the agent command
// line names them - mcp__<server>__<tool> - joined for an allowed tool list.
func QualifiedNames(server string, names ...string) string {
	qualified := make([]string, 0, len(names))
	for _, name := range names {
		qualified = append(qualified, "mcp__"+server+"__"+name)
	}

	return strings.Join(qualified, ",")
}
