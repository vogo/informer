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
	"os"
	"slices"
	"strings"

	"github.com/vogo/informer/internal/configstore"
)

// webhookKey is the top level key holding the bot webhook inside informer.json.
// It is a plain delivery address rather than a credential, so it lives next to
// the other shareable settings and never enters informer.secret.json.
const webhookKey = "webhook"

// SaveWebhook stores the bot webhook in informer.json.
// An empty value clears it. A webhook left behind in the legacy credential file
// is removed on every save so an installation that upgrades does not keep two
// copies of the same address.
func (s *Service) SaveWebhook(webhook string) error {
	trimmed := strings.TrimSpace(webhook)

	err := configstore.WithLock(s.ConfigFilePath(), configstore.DefaultLockTimeout, func() error {
		doc, loadErr := configstore.Load(s.ConfigFilePath())
		if loadErr != nil {
			return loadErr
		}

		if trimmed == "" {
			doc.Delete(webhookKey)
		} else {
			setErr := doc.Set(webhookKey, trimmed)
			if setErr != nil {
				return setErr
			}
		}

		data, bytesErr := doc.Bytes()
		if bytesErr != nil {
			return bytesErr
		}

		return configstore.WriteAtomic(s.ConfigFilePath(), data, configstore.PermConfig)
	})
	if err != nil {
		return err
	}

	return s.clearLegacyWebhook()
}

// ResolveWebhook decides which bot webhook one inform run delivers to.
//
// An address passed on the command line always wins, so every existing crontab entry
// keeps working exactly as before; informer.json is the fallback used by a run
// started without an argument. A webhook still sitting in the legacy credential
// file is honored until the next save migrates it out.
func (s *Service) ResolveWebhook(argAddr string) string {
	if trimmed := strings.TrimSpace(argAddr); trimmed != "" {
		return trimmed
	}

	webhook, err := s.readWebhook()
	if err != nil {
		// a broken configuration must not abort the daily run: the report is still
		// generated and stored, it is only not delivered.
		return ""
	}

	return webhook
}

// readWebhook returns the stored bot address from informer.json, falling back to
// the legacy credential file for installations that have not saved since the move.
func (s *Service) readWebhook() (string, error) {
	doc, err := configstore.Load(s.ConfigFilePath())
	if err != nil {
		return "", err
	}

	var webhook string

	_, err = doc.Unmarshal(webhookKey, &webhook)
	if err != nil {
		return "", err
	}

	webhook = strings.TrimSpace(webhook)
	if webhook != "" {
		return webhook, nil
	}

	return s.readLegacyWebhook(), nil
}

// readLegacyWebhook returns a webhook still stored in informer.secret.json.
// A missing or broken credential file means "no legacy address": it must not
// block reading the shareable configuration.
func (s *Service) readLegacyWebhook() string {
	doc, err := configstore.Load(s.SecretsFilePath())
	if err != nil {
		return ""
	}

	var webhook string

	_, err = doc.Unmarshal(webhookKey, &webhook)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(webhook)
}

// clearLegacyWebhook drops the webhook key from the credential file when one is
// still present. Failures are ignored: the address already lives in informer.json,
// and a later save can try again.
func (s *Service) clearLegacyWebhook() error {
	path := s.SecretsFilePath()

	_, err := os.Stat(path)
	if err != nil {
		return nil //nolint:nilerr //missing or unreadable secret file means nothing to migrate.
	}

	return configstore.WithLock(path, configstore.DefaultLockTimeout, func() error {
		return removeLegacyWebhookKey(path)
	})
}

// removeLegacyWebhookKey deletes the webhook key from path when present.
func removeLegacyWebhookKey(path string) error {
	doc, err := configstore.Load(path)
	if err != nil {
		return nil //nolint:nilerr //legacy cleanup is best effort after the config write.
	}

	if !slices.Contains(doc.Keys(), webhookKey) {
		return nil
	}

	doc.Delete(webhookKey)

	// an empty credential file is removed rather than rewritten as {}, so an
	// installation that only ever stored a webhook does not keep a useless file.
	if len(doc.Keys()) == 0 {
		removeErr := os.Remove(path)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}

		return nil
	}

	data, err := doc.Bytes()
	if err != nil {
		return err
	}

	return configstore.WriteAtomic(path, data, configstore.PermSecret)
}
