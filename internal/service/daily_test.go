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

package service_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/service"
)

func TestDailyIndexIsEmptyWithoutDataDirectory(t *testing.T) {
	svc := newService(t)

	years, err := svc.DailyIndex()
	require.NoError(t, err)
	assert.Empty(t, years)
}

func TestDailyIndexGroupsByYearAndMonthNewestFirst(t *testing.T) {
	svc := newService(t)

	writeDaily(t, svc, "2025-12-30", "old")
	writeDaily(t, svc, "2026-01-02", "january second")
	writeDaily(t, svc, "2026-01-31", "january last")
	writeDaily(t, svc, "2026-02-01", "february")

	// entries that are not daily reports are ignored instead of breaking the index.
	require.NoError(t, os.WriteFile(dailyDir(svc, "2026")+"/notes.txt", []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(dailyDir(svc, "2026")+"/2026-13-40.md", []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(svc.HomeDir(), "data", "archive"), 0o750))

	years, err := svc.DailyIndex()
	require.NoError(t, err)
	require.Len(t, years, 2)

	assert.Equal(t, "2026", years[0].Year)
	require.Len(t, years[0].Months, 2)
	assert.Equal(t, "2026-02", years[0].Months[0].Month)
	assert.Equal(t, "2026-01", years[0].Months[1].Month)

	january := years[0].Months[1].Days
	require.Len(t, january, 2)
	assert.Equal(t, "2026-01-31", january[0].Date)
	assert.Equal(t, "2026-01-02", january[1].Date)
	assert.Positive(t, january[0].Size)

	assert.Equal(t, "2025", years[1].Year)
}

func TestDailyContentReadsOneDay(t *testing.T) {
	svc := newService(t)
	writeDaily(t, svc, "2026-03-05", "# daily\n\n- one article, https://example.com/a\n")

	content, err := svc.DailyContent("2026-03-05")
	require.NoError(t, err)
	assert.Contains(t, content, "https://example.com/a")
}

func TestDailyContentReportsMissingDay(t *testing.T) {
	svc := newService(t)

	_, err := svc.DailyContent("2026-03-05")
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestDailyContentRefusesNonCanonicalDates(t *testing.T) {
	svc := newService(t)

	for _, date := range []string{
		"",
		"2026-3-5",
		"2026/03/05",
		"../../etc/passwd",
		"2026-03-05/../../../etc/passwd",
		"2026-13-01",
		"2026-03-05.md",
	} {
		_, err := svc.DailyContent(date)
		require.ErrorIsf(t, err, service.ErrInvalidArgument, "date %q must be refused", date)
	}
}

// dailyDir is the year directory of one data home.
func dailyDir(svc *service.Service, year string) string {
	return filepath.Join(svc.HomeDir(), "data", year)
}

// writeDaily stores one daily markdown report in the layout an inform run writes.
func writeDaily(t *testing.T, svc *service.Service, date, content string) {
	t.Helper()

	dir := dailyDir(svc, date[:4])
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, date+".md"), []byte(content), 0o600))
}
