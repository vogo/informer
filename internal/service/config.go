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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/inform"
)

// CreateConfig stores a new feed configuration record.
func (s *Service) CreateConfig(config *feed.Config) error {
	if config == nil {
		return fmt.Errorf("%w: config is nil", ErrInvalidArgument)
	}

	if err := s.db.Create(config).Error; err != nil {
		return fmt.Errorf("create config: %w", err)
	}

	return nil
}

// GetConfig reads one configuration record by id.
func (s *Service) GetConfig(id int64) (*feed.Config, error) {
	var config feed.Config

	if err := s.db.Where("id = ?", id).First(&config).Error; err != nil {
		return nil, wrapFind(err, "config", id)
	}

	return &config, nil
}

// UpdateConfig replaces every column of an existing configuration record.
func (s *Service) UpdateConfig(config *feed.Config) error {
	if config == nil || config.ID == 0 {
		return fmt.Errorf("%w: config id is required", ErrInvalidArgument)
	}

	if _, err := s.GetConfig(config.ID); err != nil {
		return err
	}

	if err := s.db.Save(config).Error; err != nil {
		return fmt.Errorf("update config %d: %w", config.ID, err)
	}

	return nil
}

// DeleteConfig removes a configuration record by id.
func (s *Service) DeleteConfig(id int64) error {
	result := s.db.Where("id = ?", id).Delete(&feed.Config{})
	if result.Error != nil {
		return fmt.Errorf("delete config %d: %w", id, result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: config %d", ErrNotFound, id)
	}

	return nil
}

// ListConfigs returns one page of configuration records ordered by id.
func (s *Service) ListConfigs(page PageRequest) (*Page[*feed.Config], error) {
	return findPage[*feed.Config](s.db.Model(&feed.Config{}), "id asc", page)
}

// ConfigFilePath is the informer.json path of the active data directory.
func (s *Service) ConfigFilePath() string {
	return filepath.Join(s.homeDir, inform.ConfigFileName)
}

// ReadFileConfig reads informer.json from the active data directory.
// It returns the parsed configuration together with the raw bytes, which the
// food order initialisation still consumes as a whole.
func (s *Service) ReadFileConfig() (*inform.Config, []byte, error) {
	path := s.ConfigFilePath()

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	var config inform.Config
	if err = json.Unmarshal(raw, &config); err != nil {
		return nil, nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	return &config, raw, nil
}

// EffectiveFeedConfig resolves the feed configuration used by a real inform run.
//
// informer.json stays authoritative so that existing crontab installations keep
// behaving exactly as before; the stored configuration records are only consulted
// when the file defines no feed section. A nil result means feeds are skipped.
func (s *Service) EffectiveFeedConfig(fileConfig *inform.Config) *feed.Config {
	if fileConfig != nil && fileConfig.Feed != nil {
		return fileConfig.Feed
	}

	stored, err := s.GetConfig(1)
	if err != nil {
		return nil
	}

	return stored
}
