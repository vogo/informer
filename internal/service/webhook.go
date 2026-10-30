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
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/vogo/informer/internal/configstore"
	"github.com/vogo/informer/internal/inform"
)

const (
	// webhooksKey is the top level key holding the bot delivery targets inside
	// informer.json. They are plain delivery addresses rather than credentials, so
	// they live next to the other shareable settings and never enter informer.secret.json.
	webhooksKey = "webhooks"

	// legacyWebhookKey is the single target key of earlier builds. It is read for
	// compatibility and removed on the next save.
	legacyWebhookKey = "webhook"
)

// WebhookView is one bot delivery target as the settings page renders it.
type WebhookView struct {
	// Name labels the target; it is derived from the address host when the file
	// leaves the name empty.
	Name string `json:"name"`

	// URL is the bot webhook address.
	URL string `json:"url"`

	// Enabled tells whether the target takes part in every run.
	Enabled bool `json:"enabled"`
}

// SaveWebhooks stores the bot delivery targets in informer.json.
// An empty list keeps the daily file only. A webhook left behind by an earlier
// build - in informer.json or in the legacy credential file - is removed on every
// save so an installation that upgrades does not keep two copies of one address.
func (s *Service) SaveWebhooks(webhooks []inform.Webhook) error {
	normalized, err := normalizeWebhooks(webhooks)
	if err != nil {
		return err
	}

	err = s.updateConfig(func(doc *configstore.Doc) error {
		setErr := doc.Set(webhooksKey, normalized)
		if setErr != nil {
			return setErr
		}

		doc.Delete(legacyWebhookKey)

		return nil
	})
	if err != nil {
		return err
	}

	return s.clearLegacyWebhook()
}

// normalizeWebhooks trims, defaults and validates the target list before it is written.
// A repeated address is refused rather than silently deduplicated: it is a typed in
// list, and one bot receiving the message twice is never what the user meant.
func normalizeWebhooks(webhooks []inform.Webhook) ([]inform.Webhook, error) {
	normalized := make([]inform.Webhook, 0, len(webhooks))
	seen := make(map[string]struct{}, len(webhooks))

	for _, webhook := range webhooks {
		address := strings.TrimSpace(webhook.URL)
		if address == "" {
			return nil, fmt.Errorf("%w: bot address is required", ErrInvalidArgument)
		}

		if _, ok := seen[address]; ok {
			return nil, fmt.Errorf("%w: duplicate bot address %q", ErrInvalidArgument, address)
		}

		seen[address] = struct{}{}

		entry := inform.Webhook{Name: strings.TrimSpace(webhook.Name), URL: address}

		// only an explicit pause is written, so an enabled target keeps a file of
		// bare name and address pairs.
		if webhook.Enabled != nil && !*webhook.Enabled {
			entry.Enabled = webhook.Enabled
		}

		normalized = append(normalized, entry)
	}

	return normalized, nil
}

// ResolveTargets decides which bots one inform run delivers to.
//
// An address passed on the command line replaces the stored list for this run, so
// every existing crontab entry keeps working exactly as before; informer.json is the
// fallback used by a run started without an argument. Disabled targets are skipped.
func (s *Service) ResolveTargets(argAddr string) []inform.Target {
	if trimmed := strings.TrimSpace(argAddr); trimmed != "" {
		return []inform.Target{{Name: webhookDisplayName(inform.Webhook{URL: trimmed}), URL: trimmed}}
	}

	webhooks, err := s.storedWebhooks()
	if err != nil {
		// a broken configuration must not abort the daily run: the report is still
		// generated and stored, it is only not delivered.
		return nil
	}

	targets := make([]inform.Target, 0, len(webhooks))

	for _, webhook := range webhooks {
		address := strings.TrimSpace(webhook.URL)
		if address == "" || !webhook.IsEnabled() {
			continue
		}

		targets = append(targets, inform.Target{Name: webhookDisplayName(webhook), URL: address})
	}

	return targets
}

// ReadWebhooks returns every stored target, disabled ones included, for the settings
// page. The name is filled from the address host when the file leaves it empty.
func (s *Service) ReadWebhooks() ([]WebhookView, error) {
	webhooks, err := s.storedWebhooks()
	if err != nil {
		return nil, err
	}

	views := make([]WebhookView, 0, len(webhooks))

	for _, webhook := range webhooks {
		address := strings.TrimSpace(webhook.URL)
		if address == "" {
			continue
		}

		views = append(views, WebhookView{
			Name:    webhookDisplayName(webhook),
			URL:     address,
			Enabled: webhook.IsEnabled(),
		})
	}

	return views, nil
}

// storedWebhooks returns the targets of informer.json, falling back to the single
// webhook of an earlier build - in informer.json or in the legacy credential file -
// for an installation that has not saved since the move.
func (s *Service) storedWebhooks() ([]inform.Webhook, error) {
	doc, err := configstore.Load(s.ConfigFilePath())
	if err != nil {
		return nil, err
	}

	var webhooks []inform.Webhook

	found, err := doc.Unmarshal(webhooksKey, &webhooks)
	if err != nil {
		return nil, err
	}

	if found {
		return webhooks, nil
	}

	var legacy string
	_, err = doc.Unmarshal(legacyWebhookKey, &legacy)
	if err == nil {
		if trimmed := strings.TrimSpace(legacy); trimmed != "" {
			return []inform.Webhook{{URL: trimmed}}, nil
		}
	}

	if legacy = s.readLegacyWebhook(); legacy != "" {
		return []inform.Webhook{{URL: legacy}}, nil
	}

	return nil, nil
}

// webhookDisplayName labels a target: its own name, or the address host when the
// file leaves the name empty.
func webhookDisplayName(webhook inform.Webhook) string {
	if name := strings.TrimSpace(webhook.Name); name != "" {
		return name
	}

	address := strings.TrimSpace(webhook.URL)

	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return address
	}

	return parsed.Host
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

	_, err = doc.Unmarshal(legacyWebhookKey, &webhook)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(webhook)
}

// clearLegacyWebhook drops the webhook key from the credential file when one is
// still present. Failures are ignored: the addresses already live in informer.json,
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

	if !slices.Contains(doc.Keys(), legacyWebhookKey) {
		return nil
	}

	doc.Delete(legacyWebhookKey)

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
