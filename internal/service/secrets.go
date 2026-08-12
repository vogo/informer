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

package service

import (
	"path/filepath"
	"strings"

	"github.com/vogo/informer/internal/configstore"
)

const (
	// SecretsFileName is the credential file of one data directory. It is kept out of
	// informer.json on purpose: informer.json is meant to be readable, diffable and
	// shareable, while the agent api key stays in its own locked-down file.
	SecretsFileName = "informer.secret.json"

	// agentAPIKeyKey is the top level key holding the agent api key.
	agentAPIKeyKey = "agent_api_key" //nolint:gosec //document key name, not a credential.
)

// SecretsView is what the settings page learns about the credential file.
type SecretsView struct {
	// Path is the absolute location of the credential file.
	Path string `json:"path"`

	// Exists reports whether the file is already on disk.
	Exists bool `json:"exists"`

	// AgentAPIKeyConfigured reports whether a non empty agent api key is stored.
	// The key itself is never returned: it is a real credential, and the page only
	// needs to know that one is set.
	AgentAPIKeyConfigured bool `json:"agent_api_key_configured"`
}

// SecretsFilePath is the credential file of the active data directory.
func (s *Service) SecretsFilePath() string {
	return filepath.Join(s.homeDir, SecretsFileName)
}

// ReadSecretsView reports the credential state for the settings page.
func (s *Service) ReadSecretsView() (*SecretsView, error) {
	path := s.SecretsFilePath()

	stored, err := s.readSecrets()
	if err != nil {
		return nil, err
	}

	return &SecretsView{
		Path:                  path,
		Exists:                stored.exists,
		AgentAPIKeyConfigured: stored.agentAPIKey != "",
	}, nil
}

// SaveAgentAPIKey stores the agent api key in the credential file.
// An empty value clears it, which is how an installation goes back to running the
// agent with the credentials its own command line is already logged in with.
func (s *Service) SaveAgentAPIKey(apiKey string) error {
	return s.saveSecret(agentAPIKeyKey, apiKey)
}

// saveSecret replaces one key of the credential file.
//
// The whole document is re-read inside the write lock so a second credential saved
// at the same time is not lost, and the result is written atomically at 0600: if the
// permission cannot be enforced the save fails and the previous file stays in place,
// so a credential is never left readable by other users of the machine.
func (s *Service) saveSecret(key, value string) error {
	path := s.SecretsFilePath()
	trimmed := strings.TrimSpace(value)

	return configstore.WithLock(path, configstore.DefaultLockTimeout, func() error {
		doc, err := configstore.Load(path)
		if err != nil {
			return err
		}

		err = doc.Set(key, trimmed)
		if err != nil {
			return err
		}

		data, err := doc.Bytes()
		if err != nil {
			return err
		}

		return configstore.WriteAtomic(path, data, configstore.PermSecret)
	})
}

// readAgentAPIKey returns the stored agent api key, empty when none is configured.
func (s *Service) readAgentAPIKey() (string, error) {
	stored, err := s.readSecrets()
	if err != nil {
		return "", err
	}

	return stored.agentAPIKey, nil
}

// storedSecrets is the credential file state one read resolved.
type storedSecrets struct {
	agentAPIKey string
	exists      bool
}

// readSecrets reads every stored credential and whether the file exists.
func (s *Service) readSecrets() (storedSecrets, error) {
	doc, err := configstore.Load(s.SecretsFilePath())
	if err != nil {
		return storedSecrets{}, err
	}

	stored := storedSecrets{exists: doc.Exists()}

	var apiKey string

	_, err = doc.Unmarshal(agentAPIKeyKey, &apiKey)
	if err != nil {
		return stored, err
	}

	stored.agentAPIKey = strings.TrimSpace(apiKey)

	return stored, nil
}
