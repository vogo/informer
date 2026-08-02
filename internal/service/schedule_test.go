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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/service"
)

func TestReadScheduleConfigReturnsDefaultsWhenAbsent(t *testing.T) {
	svc := newService(t)

	// the helper's informer.json has a feed section but no schedule section.
	schedule, err := svc.ReadScheduleConfig()
	require.NoError(t, err)
	assert.Equal(t, *service.DefaultScheduleConfig(), *schedule)
	assert.False(t, schedule.Enabled)
	assert.Equal(t, "10:00", schedule.Time)

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.Nil(t, view.Schedule)
}

func TestSaveScheduleConfigRoundTripsThroughTheFile(t *testing.T) {
	svc := newService(t)

	schedule := &service.Schedule{Enabled: true, Time: "09:30"}
	require.NoError(t, svc.SaveScheduleConfig(schedule))

	stored, err := svc.ReadScheduleConfig()
	require.NoError(t, err)
	assert.Equal(t, *schedule, *stored)

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	require.NotNil(t, view.Schedule)
	assert.Equal(t, *schedule, *view.Schedule)

	// the file spells the section out in its plain json shape.
	raw := readConfigFile(t, svc)
	section, ok := raw["schedule"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, section["enabled"])
	assert.Equal(t, "09:30", section["time"])

	// the feed section the helper wrote stays untouched.
	require.NotNil(t, view.Feed)
	assert.Equal(t, 10, view.Feed.MaxInformFeedSize)
}

func TestSaveScheduleConfigKeepsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeRaw(dir, `{
  "annotation": "a hand written note",
  "feed": {"max_inform_feed_size": 5, "feed_expire_days": 10, "same_site_max_count": 1, "max_fetch_num": 0},
  "schedule": {"enabled": true, "time": "08:00"},
  "prototype": {"toggle": true}
}`))

	svc, err := service.New(dir)
	require.NoError(t, err)

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.Equal(t, []string{"annotation", "prototype"}, view.PreservedKeys)
	require.NotNil(t, view.Schedule)
	assert.Equal(t, "08:00", view.Schedule.Time)

	require.NoError(t, svc.SaveScheduleConfig(&service.Schedule{Enabled: false, Time: "22:15"}))

	raw := readConfigFile(t, svc)
	assert.Equal(t, "a hand written note", raw["annotation"])
	assert.Equal(t, map[string]any{"toggle": true}, raw["prototype"])

	// the feed section survives a schedule save, and the other way around.
	feed, ok := raw["feed"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 5, feed["max_inform_feed_size"], 0)

	schedule, ok := raw["schedule"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, schedule["enabled"])
	assert.Equal(t, "22:15", schedule["time"])
}

func TestSaveScheduleConfigRejectsTimesTheSchedulerCouldNotActOn(t *testing.T) {
	svc := newService(t)

	for _, at := range []string{
		"",       // empty is not a clock time.
		"25:00",  // the hour must stay inside the day.
		"10:60",  // the minute must stay inside the hour.
		"7:5",    // the picker always sends two digits per field.
		"ab:cd",  // not numbers at all.
		"1000",   // not a clock shape.
		" 10:00", // no surrounding blanks either.
	} {
		require.ErrorIs(t, svc.SaveScheduleConfig(&service.Schedule{Enabled: true, Time: at}), service.ErrInvalidArgument, "time %q", at)
	}

	require.ErrorIs(t, svc.SaveScheduleConfig(nil), service.ErrInvalidArgument)

	// a refused save never rewrote the file: there is still no schedule section.
	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.Nil(t, view.Schedule)
}
