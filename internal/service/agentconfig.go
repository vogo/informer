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
	"strings"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/configstore"
	"github.com/vogo/informer/internal/inform"
)

// agentSectionKey is the top level key of the agent section inside informer.json.
const agentSectionKey = "agent"

// DefaultAgentConfig is the agent section a data directory without one starts from.
// Every endpoint field is empty on purpose: an installation that is already logged
// in through the agent's own command line then keeps running on that login, and
// only a user who wants a different endpoint has to fill anything in.
func DefaultAgentConfig() *agent.Config {
	return agent.DefaultConfig()
}

// agentSection is the agent part of informer.json exactly as the file spells it.
// The api key is deliberately absent: it lives in the 0600 credential file next to
// the bot webhook, so informer.json stays a document that can be shared and diffed.
type agentSection struct {
	Provider       string `json:"provider"`
	BaseURL        string `json:"base_url"`
	Model          string `json:"model"`
	AllowedTools   string `json:"allowed_tools"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Command        string `json:"command"`
}

// ReadAgentConfig reads the agent section of informer.json.
// A missing file or a missing section is reported as the defaults rather than a
// failure, so a fetch always has a well formed configuration to run with.
func (s *Service) ReadAgentConfig() (*agent.Config, error) {
	doc, err := configstore.Load(s.ConfigFilePath())
	if err != nil {
		return nil, err
	}

	var section agentSection

	found, err := doc.Unmarshal(agentSectionKey, &section)
	if err != nil {
		return nil, err
	}

	if !found {
		return DefaultAgentConfig(), nil
	}

	return &agent.Config{
		Provider:       section.Provider,
		BaseURL:        section.BaseURL,
		Model:          section.Model,
		AllowedTools:   section.AllowedTools,
		TimeoutSeconds: section.TimeoutSeconds,
		Command:        section.Command,
	}, nil
}

// SaveAgentConfig validates and stores the agent section of informer.json.
//
// The api key is not part of it: it is saved separately through SaveAgentAPIKey,
// so a page that only ever shows whether a credential exists can save the endpoint
// and the model without having to send the key back it never displayed.
func (s *Service) SaveAgentConfig(config *agent.Config) error {
	if config == nil {
		return fmt.Errorf("%w: agent config is nil", ErrInvalidArgument)
	}

	err := ValidateAgentConfig(config)
	if err != nil {
		return err
	}

	return s.saveConfigSection(agentSectionKey, agentSection{
		Provider:       strings.TrimSpace(config.Provider),
		BaseURL:        strings.TrimSpace(config.BaseURL),
		Model:          strings.TrimSpace(config.Model),
		AllowedTools:   strings.TrimSpace(config.AllowedTools),
		TimeoutSeconds: config.TimeoutSeconds,
		Command:        strings.TrimSpace(config.Command),
	})
}

// ValidateAgentConfig refuses an agent section a run could not act on.
func ValidateAgentConfig(config *agent.Config) error {
	if config == nil {
		return fmt.Errorf("%w: agent config is nil", ErrInvalidArgument)
	}

	provider := strings.TrimSpace(config.Provider)
	if provider != "" && !agent.IsLegalProvider(provider) {
		return fmt.Errorf("%w: unknown agent provider %q", ErrInvalidArgument, provider)
	}

	// zero is the documented "use the default window" value of timeout_seconds.
	if config.TimeoutSeconds == 0 {
		return nil
	}

	return checkRange("timeout_seconds", config.TimeoutSeconds, agent.MinTimeoutSeconds, agent.MaxTimeoutSeconds)
}

// EffectiveAgentConfig resolves the configuration every agent source of one run uses.
//
// The file section decides the endpoint and model, the credential file supplies the
// api key, and the data directory becomes the working directory of the agent process
// so it never inherits whatever directory the caller happened to start in.
func (s *Service) EffectiveAgentConfig(fileConfig *inform.Config) *agent.Config {
	config := DefaultAgentConfig()

	if fileConfig != nil && fileConfig.Agent != nil {
		config = fileConfig.Agent
	}

	resolved := config.Normalized()
	resolved.WorkDir = s.homeDir

	key, err := s.readAgentAPIKey()
	if err == nil && key != "" {
		resolved.APIKey = key
	}

	return resolved
}
