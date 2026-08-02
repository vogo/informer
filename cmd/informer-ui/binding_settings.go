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

package main

import (
	"fmt"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// FeedConfigDTO is the editable feed section of informer.json.
type FeedConfigDTO struct {
	MaxInformFeedSize int `json:"maxInformFeedSize"`
	FeedExpireDays    int `json:"feedExpireDays"`
	SameSiteMaxCount  int `json:"sameSiteMaxCount"`
	MaxFetchNum       int `json:"maxFetchNum"`
}

// ConfigViewDTO is the whole settings page state of informer.json.
type ConfigViewDTO struct {
	// Path is where the file lives, so the user can also edit it by hand.
	Path string `json:"path"`

	// Exists is false on a data directory that has no informer.json yet; the page
	// then shows Defaults and the first save creates the file.
	Exists bool `json:"exists"`

	// Feed is the stored section, or the defaults when the file has none.
	Feed *FeedConfigDTO `json:"feed"`

	// Defaults are the documented starting values.
	Defaults *FeedConfigDTO `json:"defaults"`

	// PreservedKeys are the top level fields this build does not edit and keeps
	// on every save, listed so the page can state that nothing is dropped.
	PreservedKeys []string `json:"preservedKeys"`
}

// SecretsViewDTO is the credential state of the settings page. The webhook is a
// plain bot address rather than a secret, so the page shows it in full.
type SecretsViewDTO struct {
	Path              string `json:"path"`
	Exists            bool   `json:"exists"`
	WebhookConfigured bool   `json:"webhookConfigured"`
	Webhook           string `json:"webhook"`
}

// HistoryIndexDTO reports what one history index rebuild did.
type HistoryIndexDTO struct {
	Days  int `json:"days"`
	Links int `json:"links"`

	// Filled is the number of articles that got their missing inform time.
	Filled int `json:"filled"`

	// Skipped is the total left untouched on purpose, broken down by the three
	// reasons below.
	Skipped               int `json:"skipped"`
	SkippedAlreadyIndexed int `json:"skippedAlreadyIndexed"`
	SkippedUnmatched      int `json:"skippedUnmatched"`
	SkippedAmbiguous      int `json:"skippedAmbiguous"`

	Failed int      `json:"failed"`
	Errors []string `json:"errors"`
}

// ReadConfig loads informer.json for the settings page.
func (a *App) ReadConfig() (*ConfigViewDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	view, err := a.svc.ReadFileConfigView()
	if err != nil {
		return nil, err
	}

	defaults := toFeedConfigDTO(service.DefaultFeedConfig())

	dto := &ConfigViewDTO{
		Path:          view.Path,
		Exists:        view.Exists,
		Feed:          defaults,
		Defaults:      defaults,
		PreservedKeys: view.PreservedKeys,
	}

	if view.Feed != nil {
		dto.Feed = toFeedConfigDTO(view.Feed)
	}

	if dto.PreservedKeys == nil {
		dto.PreservedKeys = []string{}
	}

	return dto, nil
}

// SaveConfig validates and stores the feed section of informer.json.
// The whole document is re-read under a write lock and replaced atomically, so a
// concurrent crontab run reads either the old or the new file, never a partial one,
// and fields this build does not know about survive the save.
func (a *App) SaveConfig(req *FeedConfigDTO) error {
	if req == nil {
		return fmt.Errorf("%w: request is nil", service.ErrInvalidArgument)
	}

	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.SaveFeedConfig(&feed.Config{
		MaxInformFeedSize: req.MaxInformFeedSize,
		FeedExpireDays:    req.FeedExpireDays,
		SameSiteMaxCount:  req.SameSiteMaxCount,
		MaxFetchNum:       req.MaxFetchNum,
	})
}

// ReadSecrets reports whether a bot webhook is configured, and where it is stored.
func (a *App) ReadSecrets() (*SecretsViewDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	view, err := a.svc.ReadSecretsView()
	if err != nil {
		return nil, err
	}

	return &SecretsViewDTO{
		Path:              view.Path,
		Exists:            view.Exists,
		WebhookConfigured: view.WebhookConfigured,
		Webhook:           view.Webhook,
	}, nil
}

// SaveWebhook stores the bot webhook in the separate 0600 credential file.
// An empty value clears it. The save fails rather than leaving the file readable by
// other users when the permission cannot be enforced.
func (a *App) SaveWebhook(webhook string) error {
	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.SaveWebhook(webhook)
}

// RebuildHistoryIndex fills the missing inform times that the stored daily reports
// can prove, and reports exactly what it did.
func (a *App) RebuildHistoryIndex() (*HistoryIndexDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	result, err := a.svc.RebuildHistoryIndex()
	if err != nil {
		return nil, err
	}

	errorList := result.Errors
	if errorList == nil {
		errorList = []string{}
	}

	return &HistoryIndexDTO{
		Days:                  result.Days,
		Links:                 result.Links,
		Filled:                result.Filled,
		Skipped:               result.Skipped(),
		SkippedAlreadyIndexed: result.SkippedAlreadyIndexed,
		SkippedUnmatched:      result.SkippedUnmatched,
		SkippedAmbiguous:      result.SkippedAmbiguous,
		Failed:                result.Failed,
		Errors:                errorList,
	}, nil
}

func toFeedConfigDTO(config *feed.Config) *FeedConfigDTO {
	return &FeedConfigDTO{
		MaxInformFeedSize: config.MaxInformFeedSize,
		FeedExpireDays:    config.FeedExpireDays,
		SameSiteMaxCount:  config.SameSiteMaxCount,
		MaxFetchNum:       config.MaxFetchNum,
	}
}
