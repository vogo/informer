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

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/configstore"
	"github.com/vogo/informer/internal/feed"
)

// feedSectionKey is the top level key of the feed section inside informer.json.
const feedSectionKey = "feed"

// Bounds the settings page enforces on the feed section. They exist to catch a typo,
// not to express taste: a zero expire window or a zero same site limit would make an
// inform run select nothing at all, and the divisions in the scoring would break.
const (
	MinFeedExpireDays    = 1
	MaxFeedExpireDays    = 36500
	MinInformFeedSize    = 1
	MaxInformFeedSizeCap = 1000
	MinSameSiteMaxCount  = 1
	MaxSameSiteMaxCount  = 1000
	MaxFetchNumCap       = 1000
)

// DefaultFeedConfig is the feed section a data directory without informer.json starts
// from. It matches the documented example configuration, so the settings page offers
// the same numbers the README tells a command line user to write by hand.
func DefaultFeedConfig() *feed.Config {
	return &feed.Config{
		SameSiteMaxCount:  3,
		FeedExpireDays:    150,
		MaxInformFeedSize: 20,
		MaxFetchNum:       1,
	}
}

// FileConfigView is the informer.json state the settings page renders.
type FileConfigView struct {
	// Path is the absolute location of the file, shown so the user can find it.
	Path string `json:"path"`

	// Exists reports whether the file is already on disk. A fresh installation has
	// no file yet, and the page then offers the defaults instead of an error.
	Exists bool `json:"exists"`

	// Feed is the editable feed section, nil when the file defines none.
	Feed *feed.Config `json:"feed"`

	// Schedule is the editable schedule section, nil when the file defines none.
	Schedule *Schedule `json:"schedule"`

	// Agent is the editable agent section, nil when the file defines none.
	Agent *agent.Config `json:"agent"`

	// PreservedKeys lists the top level keys this build does not edit but keeps on
	// save, so the page can state plainly that nothing else is dropped.
	PreservedKeys []string `json:"preserved_keys"`
}

// feedSection is the feed part of informer.json exactly as the file spells it.
// It deliberately excludes the database id that the stored fallback record carries:
// the file is configuration, not a table row.
type feedSection struct {
	SameSiteMaxCount  int `json:"same_site_max_count"`
	FeedExpireDays    int `json:"feed_expire_days"`
	MaxInformFeedSize int `json:"max_inform_feed_size"`
	MaxFetchNum       int `json:"max_fetch_num"`
}

// ReadFileConfigView reads informer.json for the settings page.
// Unlike ReadFileConfig, which backs a real inform run, a missing file is reported as
// a not yet created configuration instead of a failure.
func (s *Service) ReadFileConfigView() (*FileConfigView, error) {
	path := s.ConfigFilePath()

	doc, err := configstore.Load(path)
	if err != nil {
		return nil, err
	}

	view := &FileConfigView{Path: path, Exists: doc.Exists(), PreservedKeys: []string{}}

	var section feedSection

	found, err := doc.Unmarshal(feedSectionKey, &section)
	if err != nil {
		return nil, err
	}

	if found {
		view.Feed = &feed.Config{
			SameSiteMaxCount:  section.SameSiteMaxCount,
			FeedExpireDays:    section.FeedExpireDays,
			MaxInformFeedSize: section.MaxInformFeedSize,
			MaxFetchNum:       section.MaxFetchNum,
		}
	}

	var schedule Schedule

	scheduleFound, err := doc.Unmarshal(scheduleSectionKey, &schedule)
	if err != nil {
		return nil, err
	}

	if scheduleFound {
		view.Schedule = &schedule
	}

	view.Agent, err = readAgentSection(doc)
	if err != nil {
		return nil, err
	}

	for _, key := range doc.Keys() {
		if key != feedSectionKey && key != scheduleSectionKey && key != agentSectionKey {
			view.PreservedKeys = append(view.PreservedKeys, key)
		}
	}

	return view, nil
}

// readAgentSection reads the agent section of one loaded document, nil when the
// file defines none.
func readAgentSection(doc *configstore.Doc) (*agent.Config, error) {
	var section agentSection

	found, err := doc.Unmarshal(agentSectionKey, &section)
	if err != nil {
		return nil, err
	}

	if !found {
		return nil, nil //nolint:nilnil //"no section" is not an error, and the caller renders it as the defaults.
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

// SaveFeedConfig validates and stores the feed section of informer.json.
//
// The whole document is re-read inside the write lock and only the feed section is
// replaced, so a field written by another build - or by hand - survives the save, and
// a second writer running at the same time cannot lose the update it just made.
func (s *Service) SaveFeedConfig(config *feed.Config) error {
	if config == nil {
		return fmt.Errorf("%w: feed config is nil", ErrInvalidArgument)
	}

	err := ValidateFeedConfig(config)
	if err != nil {
		return err
	}

	return s.saveConfigSection(feedSectionKey, feedSection{
		SameSiteMaxCount:  config.SameSiteMaxCount,
		FeedExpireDays:    config.FeedExpireDays,
		MaxInformFeedSize: config.MaxInformFeedSize,
		MaxFetchNum:       config.MaxFetchNum,
	})
}

// saveConfigSection stores one top level section of informer.json.
//
// The whole document is re-read inside the write lock and only the named section is
// replaced, so a field written by another build - or by hand - survives the save, and
// a second writer running at the same time cannot lose the update it just made.
func (s *Service) saveConfigSection(key string, section any) error {
	path := s.ConfigFilePath()

	return configstore.WithLock(path, configstore.DefaultLockTimeout, func() error {
		doc, err := configstore.Load(path)
		if err != nil {
			return err
		}

		setErr := doc.Set(key, section)
		if setErr != nil {
			return setErr
		}

		data, err := doc.Bytes()
		if err != nil {
			return err
		}

		return configstore.WriteAtomic(path, data, configstore.PermConfig)
	})
}

// ValidateFeedConfig refuses a feed section that would break an inform run.
func ValidateFeedConfig(config *feed.Config) error {
	err := checkRange("max_inform_feed_size", config.MaxInformFeedSize, MinInformFeedSize, MaxInformFeedSizeCap)
	if err != nil {
		return err
	}

	err = checkRange("feed_expire_days", config.FeedExpireDays, MinFeedExpireDays, MaxFeedExpireDays)
	if err != nil {
		return err
	}

	err = checkRange("same_site_max_count", config.SameSiteMaxCount, MinSameSiteMaxCount, MaxSameSiteMaxCount)
	if err != nil {
		return err
	}

	// zero is the documented "no global limit" value of max_fetch_num.
	return checkRange("max_fetch_num", config.MaxFetchNum, 0, MaxFetchNumCap)
}

func checkRange(name string, value, low, high int) error {
	if value < low || value > high {
		return fmt.Errorf("%w: %s must be between %d and %d, got %d", ErrInvalidArgument, name, low, high, value)
	}

	return nil
}
