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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vogo/informer/internal/inform"
)

// monthLayout groups the days of one daily index under a year and month key.
const monthLayout = "2006-01"

// DailyDay is one stored daily report.
type DailyDay struct {
	// Date is the day in 2006-01-02 form, the key DailyContent takes.
	Date string `json:"date"`

	// Size is the byte size of the stored markdown file.
	Size int64 `json:"size"`
}

// DailyMonth is one month of the daily index, days newest first.
type DailyMonth struct {
	// Month is the 2006-01 key of the group.
	Month string `json:"month"`

	Days []*DailyDay `json:"days"`
}

// DailyYear is one year of the daily index, months newest first.
type DailyYear struct {
	// Year is the 2006 key of the group.
	Year string `json:"year"`

	Months []*DailyMonth `json:"months"`
}

// DailyIndex lists every stored daily report grouped by year and month, newest first.
//
// It is a directory listing, not a content scan: only the file names decide what a day
// is, so a directory holding thousands of days stays cheap to browse. A missing data
// directory is an empty index, not a failure - a fresh installation simply has no
// reports yet.
func (s *Service) DailyIndex() ([]*DailyYear, error) {
	root := filepath.Join(s.homeDir, inform.DataDirName)

	yearDirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []*DailyYear{}, nil
		}

		return nil, fmt.Errorf("read daily data directory %q: %w", root, err)
	}

	years := make([]*DailyYear, 0, len(yearDirs))

	for _, yearDir := range yearDirs {
		if !yearDir.IsDir() {
			continue
		}

		year := yearDir.Name()
		if !isYearDirName(year) {
			continue
		}

		months, err := readDailyYear(filepath.Join(root, year), year)
		if err != nil {
			return nil, err
		}

		if len(months) > 0 {
			years = append(years, &DailyYear{Year: year, Months: months})
		}
	}

	// newest year first, the order the browser opens on.
	sort.Slice(years, func(i, j int) bool { return years[i].Year > years[j].Year })

	return years, nil
}

// DailyContent returns the markdown of one stored daily report.
//
// The date is the only caller supplied part of the path, and it has to round trip
// through the canonical layout before it is used, so no separator, no "..", and no
// alternative spelling of a date can ever reach the file system.
func (s *Service) DailyContent(date string) (string, error) {
	day, err := parseDay(date)
	if err != nil {
		return "", err
	}

	path := inform.DailyFilePath(s.homeDir, day)

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: daily report %s", ErrNotFound, date)
		}

		return "", fmt.Errorf("read daily report %s: %w", date, err)
	}

	return string(raw), nil
}

// readDailyYear groups the daily files of one year directory by month, newest first.
func readDailyYear(dir, year string) ([]*DailyMonth, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read daily year directory %q: %w", dir, err)
	}

	byMonth := make(map[string][]*DailyDay)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		date, ok := dailyFileDate(entry.Name(), year)
		if !ok {
			continue
		}

		var size int64

		info, statErr := entry.Info()
		if statErr == nil {
			size = info.Size()
		}

		month := date[:len(monthLayout)]
		byMonth[month] = append(byMonth[month], &DailyDay{Date: date, Size: size})
	}

	months := make([]*DailyMonth, 0, len(byMonth))

	for month, days := range byMonth {
		sort.Slice(days, func(i, j int) bool { return days[i].Date > days[j].Date })
		months = append(months, &DailyMonth{Month: month, Days: days})
	}

	sort.Slice(months, func(i, j int) bool { return months[i].Month > months[j].Month })

	return months, nil
}

// dailyFileDate reports the date of one daily file name, and whether the name is a
// daily report of the given year at all. A file whose name parses to another year is
// rejected: it would not be reachable through DailyContent, which derives the year
// directory from the date.
func dailyFileDate(name, year string) (string, bool) {
	date := strings.TrimSuffix(name, inform.DailyFileExt)
	if date == name {
		return "", false
	}

	day, err := parseDay(date)
	if err != nil {
		return "", false
	}

	if day.Format(inform.YearLayout) != year {
		return "", false
	}

	return date, true
}

// isYearDirName reports whether the directory name is a four digit year.
func isYearDirName(name string) bool {
	if len(name) != len(inform.YearLayout) {
		return false
	}

	for _, r := range name {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

// parseDay accepts exactly the canonical 2006-01-02 spelling of a date.
//
// The local time zone is deliberate: an inform run names its file with the local
// date, so reading one back has to interpret it the same way.
func parseDay(date string) (time.Time, error) {
	day, err := time.ParseInLocation(inform.DayLayout, date, time.Local) //nolint:gosmopolitan //see above.
	if err != nil || day.Format(inform.DayLayout) != date {
		return time.Time{}, fmt.Errorf("%w: %q is not a %s date", ErrInvalidArgument, date, inform.DayLayout)
	}

	return day, nil
}
