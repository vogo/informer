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
	// shareable, a bot webhook is not.
	SecretsFileName = "informer.secret.json"

	// webhookKey is the top level key holding the bot webhook.
	webhookKey = "webhook"

	// maskKeepChars is how many leading characters of a webhook stay visible when the
	// settings page shows which credential is configured.
	maskKeepChars = 24
)

// SecretsView is what the settings page learns about the credential file.
// The webhook itself never leaves the backend: the page only sees whether one is
// configured and a masked hint identifying it.
type SecretsView struct {
	// Path is the absolute location of the credential file.
	Path string `json:"path"`

	// Exists reports whether the file is already on disk.
	Exists bool `json:"exists"`

	// WebhookConfigured reports whether a non empty webhook is stored.
	WebhookConfigured bool `json:"webhook_configured"`

	// WebhookMasked identifies the stored webhook without revealing its token.
	WebhookMasked string `json:"webhook_masked"`
}

// SecretsFilePath is the credential file of the active data directory.
func (s *Service) SecretsFilePath() string {
	return filepath.Join(s.homeDir, SecretsFileName)
}

// ReadSecretsView reports the credential state for the settings page.
func (s *Service) ReadSecretsView() (*SecretsView, error) {
	path := s.SecretsFilePath()

	stored, err := s.readWebhook()
	if err != nil {
		return nil, err
	}

	return &SecretsView{
		Path:              path,
		Exists:            stored.exists,
		WebhookConfigured: stored.webhook != "",
		WebhookMasked:     maskSecret(stored.webhook),
	}, nil
}

// SaveWebhook stores the bot webhook in the credential file.
//
// An empty value clears the stored webhook rather than writing a blank one. The file
// is written atomically and its permission is verified afterwards: if it cannot be
// brought to 0600, the save fails and the previous file is left untouched, so a
// credential is never left readable by other users of the machine.
func (s *Service) SaveWebhook(webhook string) error {
	path := s.SecretsFilePath()
	value := strings.TrimSpace(webhook)

	return configstore.WithLock(path, configstore.DefaultLockTimeout, func() error {
		doc, err := configstore.Load(path)
		if err != nil {
			return err
		}

		err = doc.Set(webhookKey, value)
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

// ResolveWebhook decides which bot webhook one inform run delivers to.
//
// An address passed on the command line always wins, so every existing crontab entry
// keeps working exactly as before; the credential file is the fallback used by a run
// started without an argument, which is how the desktop app configures delivery.
func (s *Service) ResolveWebhook(argAddr string) string {
	if trimmed := strings.TrimSpace(argAddr); trimmed != "" {
		return trimmed
	}

	stored, err := s.readWebhook()
	if err != nil {
		// a broken credential file must not abort the daily run: the report is still
		// generated and stored, it is only not delivered.
		return ""
	}

	return stored.webhook
}

// storedSecrets is the credential file state one read resolved.
type storedSecrets struct {
	webhook string
	exists  bool
}

// readWebhook reads the stored webhook and whether the credential file exists.
func (s *Service) readWebhook() (storedSecrets, error) {
	doc, err := configstore.Load(s.SecretsFilePath())
	if err != nil {
		return storedSecrets{}, err
	}

	var webhook string

	_, err = doc.Unmarshal(webhookKey, &webhook)
	if err != nil {
		return storedSecrets{exists: doc.Exists()}, err
	}

	return storedSecrets{webhook: strings.TrimSpace(webhook), exists: doc.Exists()}, nil
}

// maskSecret keeps the identifying prefix of a credential and hides the rest.
func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}

	runes := []rune(secret)
	if len(runes) <= maskKeepChars {
		return strings.Repeat("*", len(runes))
	}

	return string(runes[:maskKeepChars]) + strings.Repeat("*", 8)
}
