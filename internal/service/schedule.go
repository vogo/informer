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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vogo/informer/internal/configstore"
)

// scheduleSectionKey is the top level key of the schedule section inside informer.json.
const scheduleSectionKey = "schedule"

// ScheduleStateFileName is the file that records the calendar day of the last
// successful desktop scheduled push. It lives next to informer.json so a restarted
// app can honour the once-per-day guard without asking the bot again.
const ScheduleStateFileName = "informer.schedule-state"

// scheduleTimeLayout is the 24 hour clock format the schedule section stores,
// Go's reference hour and minute.
const scheduleTimeLayout = "15:04"

// scheduleDateLayout is the calendar day written into the schedule state file.
const scheduleDateLayout = "2006-01-02"

// Schedule is the daily push schedule of the desktop app. The command line entry
// never reads it: a CLI run stays scheduled by the operator's crontab, while the
// desktop app runs its own scheduler on this section and ignores none of it.
type Schedule struct {
	// Enabled turns the desktop scheduler on. A disabled schedule never fires,
	// whatever Time says.
	Enabled bool `json:"enabled"`

	// Time is the local wall clock time of the one daily run, "HH:MM" layout.
	Time string `json:"time"`
}

// DefaultScheduleConfig is the schedule a data directory without a schedule section
// starts from. It is switched off so a fresh installation never pushes on its own,
// and carries the documented example hour so the settings page has a value to show.
func DefaultScheduleConfig() *Schedule {
	return &Schedule{
		Enabled: false,
		Time:    "10:00",
	}
}

// ReadScheduleConfig reads the schedule section of informer.json.
// Unlike ReadFileConfig, which backs a real inform run, a missing file or a missing
// section is reported as the defaults instead of a failure, so the desktop scheduler
// always has a well formed value to act on.
func (s *Service) ReadScheduleConfig() (*Schedule, error) {
	doc, err := configstore.Load(s.ConfigFilePath())
	if err != nil {
		return nil, err
	}

	schedule := DefaultScheduleConfig()

	_, err = doc.Unmarshal(scheduleSectionKey, schedule)
	if err != nil {
		return nil, err
	}

	return schedule, nil
}

// SaveScheduleConfig validates and stores the schedule section of informer.json.
//
// As with the feed section, the whole document is re-read inside the write lock and
// only the schedule section is replaced, so every other field - including one this
// build does not know about - survives the save.
func (s *Service) SaveScheduleConfig(schedule *Schedule) error {
	if schedule == nil {
		return fmt.Errorf("%w: schedule config is nil", ErrInvalidArgument)
	}

	err := ValidateScheduleConfig(schedule)
	if err != nil {
		return err
	}

	return s.saveConfigSection(scheduleSectionKey, schedule)
}

// ValidateScheduleConfig refuses a schedule the desktop scheduler could not act on:
// the time must be a full 24 hour "HH:MM" value between 00:00 and 23:59.
func ValidateScheduleConfig(schedule *Schedule) error {
	_, err := time.Parse(scheduleTimeLayout, schedule.Time)
	if err != nil {
		return fmt.Errorf("%w: schedule time must be HH:MM between 00:00 and 23:59, got %q", ErrInvalidArgument, schedule.Time)
	}

	return nil
}

// ScheduleStatePath is the once-per-day state file of the active data directory.
func (s *Service) ScheduleStatePath() string {
	return filepath.Join(s.homeDir, ScheduleStateFileName)
}

// ReadScheduleLastRunDate returns the calendar day of the last successful desktop
// scheduled push, or an empty string when none is stored yet. A missing or blank
// file is not an error: the scheduler then treats today as not yet pushed.
func (s *Service) ReadScheduleLastRunDate() (string, error) {
	data, err := os.ReadFile(s.ScheduleStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("read schedule state file: %w", err)
	}

	day := strings.TrimSpace(string(data))
	if day == "" {
		return "", nil
	}

	_, err = time.Parse(scheduleDateLayout, day)
	if err != nil {
		// a corrupt marker must not freeze the scheduler forever: pretend there is
		// no prior run and let the next success overwrite the file.
		return "", nil
	}

	return day, nil
}

// MarkScheduleLastRunDate records that the desktop scheduler successfully pushed
// on the given calendar day. The next process start reads this file and skips the
// catch-up fire for that day.
func (s *Service) MarkScheduleLastRunDate(day string) error {
	_, err := time.Parse(scheduleDateLayout, day)
	if err != nil {
		return fmt.Errorf("%w: schedule last run date must be YYYY-MM-DD, got %q", ErrInvalidArgument, day)
	}

	return configstore.WriteAtomic(s.ScheduleStatePath(), []byte(day+"\n"), configstore.PermConfig)
}
