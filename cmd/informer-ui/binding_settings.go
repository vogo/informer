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

	"github.com/vogo/informer/internal/agent"
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

// AgentConfigDTO is the editable agent section of informer.json. It carries no
// api key: the credential lives in the separate 0600 file and is saved through
// SaveAgentAPIKey, so the settings page never has to hold it to save the rest.
type AgentConfigDTO struct {
	Provider       string `json:"provider"`
	BaseURL        string `json:"baseURL"`
	Model          string `json:"model"`
	AllowedTools   string `json:"allowedTools"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	Command        string `json:"command"`
}

// ScheduleDTO is the editable schedule section of informer.json.
type ScheduleDTO struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"`
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

	// Schedule is the stored section, or the defaults when the file has none.
	Schedule *ScheduleDTO `json:"schedule"`

	// Defaults are the documented starting values of the feed section.
	Defaults *FeedConfigDTO `json:"defaults"`

	// ScheduleDefaults are the documented starting values of the schedule section.
	ScheduleDefaults *ScheduleDTO `json:"scheduleDefaults"`

	// Agent is the stored section, or the defaults when the file has none.
	Agent *AgentConfigDTO `json:"agent"`

	// AgentDefaults are the starting values of the agent section.
	AgentDefaults *AgentConfigDTO `json:"agentDefaults"`

	// AgentProviders are the providers the agent section accepts, in order.
	AgentProviders []string `json:"agentProviders"`

	// Webhook is the bot delivery address stored in informer.json. It is a plain
	// endpoint rather than a secret, so the settings page shows it in full.
	Webhook string `json:"webhook"`

	// HTTPProxy is the optional HTTP(S) proxy stored in informer.json.
	HTTPProxy string `json:"httpProxy"`

	// PreservedKeys are the top level fields this build does not edit and keeps
	// on every save, listed so the page can state that nothing is dropped.
	PreservedKeys []string `json:"preservedKeys"`
}

// SecretsViewDTO is the credential state of the settings page.
type SecretsViewDTO struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`

	// AgentAPIKeyConfigured reports whether an agent api key is stored. The key
	// itself is never sent to the page: it is a real credential, and the page
	// only needs to know that one exists.
	AgentAPIKeyConfigured bool `json:"agentApiKeyConfigured"`
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
	scheduleDefaults := toScheduleDTO(service.DefaultScheduleConfig())
	agentDefaults := toAgentConfigDTO(service.DefaultAgentConfig())

	dto := &ConfigViewDTO{
		Path:             view.Path,
		Exists:           view.Exists,
		Feed:             defaults,
		Schedule:         scheduleDefaults,
		Defaults:         defaults,
		ScheduleDefaults: scheduleDefaults,
		Agent:            agentDefaults,
		AgentDefaults:    agentDefaults,
		AgentProviders:   agent.LegalProviders(),
		Webhook:          view.Webhook,
		HTTPProxy:        view.HTTPProxy,
		PreservedKeys:    view.PreservedKeys,
	}

	if view.Feed != nil {
		dto.Feed = toFeedConfigDTO(view.Feed)
	}

	if view.Schedule != nil {
		dto.Schedule = toScheduleDTO(view.Schedule)
	}

	if view.Agent != nil {
		dto.Agent = toAgentConfigDTO(view.Agent)
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

// ReadSecrets reports whether an agent api key is configured, and where it is stored.
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
		Path:                  view.Path,
		Exists:                view.Exists,
		AgentAPIKeyConfigured: view.AgentAPIKeyConfigured,
	}, nil
}

// SaveWebhook stores the bot webhook in informer.json.
// An empty value clears it. A webhook left in the legacy credential file is removed
// on save so the address is not kept in two places.
func (a *App) SaveWebhook(webhook string) error {
	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.SaveWebhook(webhook)
}

// SaveHTTPProxy stores the HTTP(S) proxy in informer.json and applies it to URL
// fetches and agent runs. An empty value clears it.
func (a *App) SaveHTTPProxy(proxy string) error {
	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.SaveHTTPProxy(proxy)
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

// SaveSchedule validates and stores the schedule section of informer.json.
// The desktop scheduler reads the file on every tick, so the new switch and time
// take effect without a restart.
func (a *App) SaveSchedule(req *ScheduleDTO) error {
	if req == nil {
		return fmt.Errorf("%w: request is nil", service.ErrInvalidArgument)
	}

	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.SaveScheduleConfig(&service.Schedule{
		Enabled: req.Enabled,
		Time:    req.Time,
	})
}

// SaveAgentConfig validates and stores the agent section of informer.json.
// Like every other section save it re-reads the whole document under a write lock,
// so a field this build does not edit survives.
func (a *App) SaveAgentConfig(req *AgentConfigDTO) error {
	if req == nil {
		return fmt.Errorf("%w: request is nil", service.ErrInvalidArgument)
	}

	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.SaveAgentConfig(&agent.Config{
		Provider:       req.Provider,
		BaseURL:        req.BaseURL,
		Model:          req.Model,
		AllowedTools:   req.AllowedTools,
		TimeoutSeconds: req.TimeoutSeconds,
		Command:        req.Command,
	})
}

// SaveAgentAPIKey stores the agent api key in the separate 0600 credential file.
// An empty value clears it, which puts the agent back on whatever credentials its
// own command line is logged in with.
func (a *App) SaveAgentAPIKey(apiKey string) error {
	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.SaveAgentAPIKey(apiKey)
}

// DetectAgentCommand locates the default executable of the given agent provider on
// this machine. The path is returned for the settings page to fill in; it is not
// written until the user saves, or until an agent run remembers an empty command.
func (a *App) DetectAgentCommand(provider string) (string, error) {
	err := a.ready()
	if err != nil {
		return "", err
	}

	return a.svc.DetectAgentCommand(provider)
}

func toAgentConfigDTO(config *agent.Config) *AgentConfigDTO {
	return &AgentConfigDTO{
		Provider:       config.Provider,
		BaseURL:        config.BaseURL,
		Model:          config.Model,
		AllowedTools:   config.AllowedTools,
		TimeoutSeconds: config.TimeoutSeconds,
		Command:        config.Command,
	}
}

func toFeedConfigDTO(config *feed.Config) *FeedConfigDTO {
	return &FeedConfigDTO{
		MaxInformFeedSize: config.MaxInformFeedSize,
		FeedExpireDays:    config.FeedExpireDays,
		SameSiteMaxCount:  config.SameSiteMaxCount,
		MaxFetchNum:       config.MaxFetchNum,
	}
}

func toScheduleDTO(schedule *service.Schedule) *ScheduleDTO {
	return &ScheduleDTO{
		Enabled: schedule.Enabled,
		Time:    schedule.Time,
	}
}
