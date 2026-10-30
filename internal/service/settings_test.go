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
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/service"
)

const (
	testWebhookURL        = "https://example.com/hook"
	developmentTargetName = "development"
)

// validFeedConfig is a feed section every validation rule accepts.
func validFeedConfig() *feed.Config {
	return &feed.Config{
		MaxInformFeedSize: 20,
		FeedExpireDays:    150,
		SameSiteMaxCount:  3,
		MaxFetchNum:       1,
	}
}

func TestReadFileConfigViewReportsAMissingFile(t *testing.T) {
	dir := t.TempDir()

	svc, err := service.New(dir)
	require.NoError(t, err)

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.False(t, view.Exists)
	assert.Nil(t, view.Feed)
	assert.Equal(t, svc.ConfigFilePath(), view.Path)
	assert.Empty(t, view.PreservedKeys)
}

func TestSaveFeedConfigRoundTripsThroughTheCLIReader(t *testing.T) {
	svc := newService(t)

	config := validFeedConfig()
	require.NoError(t, svc.SaveFeedConfig(config))

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	require.NotNil(t, view.Feed)
	assert.True(t, view.Exists)
	assert.Equal(t, *config, *view.Feed)

	// the CLI reads the very same file through its own path and acts on it.
	fileConfig, err := svc.ReadFileConfig()
	require.NoError(t, err)
	require.NotNil(t, fileConfig.Feed)
	assert.Equal(t, config.MaxInformFeedSize, fileConfig.Feed.MaxInformFeedSize)
	assert.Equal(t, config.FeedExpireDays, svc.EffectiveFeedConfig(fileConfig).FeedExpireDays)
}

func TestSaveFeedConfigKeepsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeRaw(dir, `{
  "comment": "a hand written note",
  "feed": {"max_inform_feed_size": 5, "feed_expire_days": 10, "same_site_max_count": 1, "max_fetch_num": 0},
  "experimental": {"flag": true}
}`))

	svc, err := service.New(dir)
	require.NoError(t, err)

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.Equal(t, []string{"comment", "experimental"}, view.PreservedKeys)

	require.NoError(t, svc.SaveFeedConfig(validFeedConfig()))

	raw := readConfigFile(t, svc)
	assert.Equal(t, "a hand written note", raw["comment"])
	assert.Equal(t, map[string]any{"flag": true}, raw["experimental"])

	// the file is not polluted with the database id of the stored fallback record.
	section, ok := raw["feed"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 20, section["max_inform_feed_size"], 0)
	assert.NotContains(t, section, "id")
}

func TestSaveFeedConfigRejectsValuesThatBreakAnInformRun(t *testing.T) {
	svc := newService(t)

	for name, mutate := range map[string]func(*feed.Config){
		"zero expire days":     func(c *feed.Config) { c.FeedExpireDays = 0 },
		"zero inform size":     func(c *feed.Config) { c.MaxInformFeedSize = 0 },
		"zero same site":       func(c *feed.Config) { c.SameSiteMaxCount = 0 },
		"negative fetch num":   func(c *feed.Config) { c.MaxFetchNum = -1 },
		"absurd inform size":   func(c *feed.Config) { c.MaxInformFeedSize = 100_000 },
		"absurd expire window": func(c *feed.Config) { c.FeedExpireDays = 1_000_000 },
	} {
		t.Run(name, func(t *testing.T) {
			config := validFeedConfig()
			mutate(config)
			require.ErrorIs(t, svc.SaveFeedConfig(config), service.ErrInvalidArgument)
		})
	}

	require.ErrorIs(t, svc.SaveFeedConfig(nil), service.ErrInvalidArgument)

	// a refused save never rewrote the file: the helper's original section is intact.
	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	require.NotNil(t, view.Feed)
	assert.Equal(t, 10, view.Feed.MaxInformFeedSize)
}

func TestSaveFeedConfigSurvivesConcurrentWriters(t *testing.T) {
	svc := newService(t)

	var wait sync.WaitGroup

	for i := range 8 {
		wait.Go(func() {
			config := validFeedConfig()
			config.MaxInformFeedSize = 10 + i
			assert.NoError(t, svc.SaveFeedConfig(config))
		})
	}

	wait.Wait()

	// whichever writer came last, the file is one complete valid document.
	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	require.NotNil(t, view.Feed)
	assert.GreaterOrEqual(t, view.Feed.MaxInformFeedSize, 10)
	assert.LessOrEqual(t, view.Feed.MaxInformFeedSize, 17)
	assert.Equal(t, 150, view.Feed.FeedExpireDays)
}

func TestSaveWebhooksWritesToInformerJSON(t *testing.T) {
	svc := newService(t)

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.Empty(t, view.Webhooks)

	const (
		dingURL = "https://oapi.dingtalk.com/robot/send?access_token=plain-bot-token"
		larkURL = "https://open.feishu.cn/open-apis/bot/v2/hook/plain-bot-token"
	)

	disabled := false

	require.NoError(t, svc.SaveWebhooks([]inform.Webhook{
		{Name: " " + developmentTargetName + " ", URL: " " + dingURL + " "},
		{URL: larkURL, Enabled: &disabled},
	}))

	view, err = svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.True(t, view.Exists)
	require.Len(t, view.Webhooks, 2)
	assert.Equal(t, developmentTargetName, view.Webhooks[0].Name)
	assert.Equal(t, dingURL, view.Webhooks[0].URL)
	assert.True(t, view.Webhooks[0].Enabled)
	assert.Equal(t, "open.feishu.cn", view.Webhooks[1].Name, "an unnamed target is labeled by its host")
	assert.Equal(t, larkURL, view.Webhooks[1].URL)
	assert.False(t, view.Webhooks[1].Enabled)

	raw := readConfigFile(t, svc)
	assert.Len(t, raw["webhooks"], 2)
	assert.NotContains(t, raw, "webhook", "the legacy key is dropped by a save")
	assert.NotContains(t, raw, "id")

	// a paused target keeps its address in the file but takes no part in a run.
	targets := svc.ResolveTargets("")
	require.Len(t, targets, 1)
	assert.Equal(t, developmentTargetName, targets[0].Name)
	assert.Equal(t, dingURL, targets[0].URL)

	// the credential file is not created for plain bot addresses.
	_, err = os.Stat(svc.SecretsFilePath())
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestResolveTargetsPrefersTheCommandLineAddress(t *testing.T) {
	svc := newService(t)

	require.NoError(t, svc.SaveWebhooks([]inform.Webhook{{URL: "https://open.feishu.cn/hook/stored"}}))

	// an explicit address replaces the stored list for this run, so crontab keeps
	// its behavior; a blank argument falls back to the file.
	targets := svc.ResolveTargets("  https://example.com/hook ")
	require.Len(t, targets, 1)
	assert.Equal(t, testWebhookURL, targets[0].URL)
	assert.Equal(t, "example.com", targets[0].Name)

	assert.Len(t, svc.ResolveTargets("   "), 1)
}

func TestSaveWebhooksRefusesABlankOrRepeatedAddress(t *testing.T) {
	svc := newService(t)

	require.ErrorIs(t, svc.SaveWebhooks([]inform.Webhook{{Name: "empty"}}), service.ErrInvalidArgument)
	require.ErrorIs(t, svc.SaveWebhooks([]inform.Webhook{
		{URL: testWebhookURL},
		{URL: testWebhookURL},
	}), service.ErrInvalidArgument)

	// a refused save never rewrote the file.
	view, err := svc.ReadWebhooks()
	require.NoError(t, err)
	assert.Empty(t, view)
}

func TestSaveWebhooksMigratesALegacySecretEntry(t *testing.T) {
	svc := newService(t)

	require.NoError(t, os.WriteFile(svc.SecretsFilePath(), []byte(`{"webhook":"https://legacy.example/hook","agent_api_key":"sk-keep"}`), 0o600))

	targets := svc.ResolveTargets("")
	require.Len(t, targets, 1)
	assert.Equal(t, "https://legacy.example/hook", targets[0].URL)

	require.NoError(t, svc.SaveWebhooks([]inform.Webhook{{URL: "https://example.com/new"}}))

	view, err := svc.ReadWebhooks()
	require.NoError(t, err)
	require.Len(t, view, 1)
	assert.Equal(t, "https://example.com/new", view[0].URL)

	raw := readConfigFile(t, svc)
	assert.NotContains(t, raw, "webhook")
	assert.Len(t, raw["webhooks"], 1)

	secretsRaw, err := os.ReadFile(svc.SecretsFilePath())
	require.NoError(t, err)

	var secrets map[string]any
	require.NoError(t, json.Unmarshal(secretsRaw, &secrets))
	assert.NotContains(t, secrets, "webhook")
	assert.Equal(t, "sk-keep", secrets["agent_api_key"])
}

func TestSaveWebhooksClearsTheStoredList(t *testing.T) {
	svc := newService(t)

	require.NoError(t, svc.SaveWebhooks([]inform.Webhook{{URL: testWebhookURL}}))
	require.NoError(t, svc.SaveWebhooks(nil))

	view, err := svc.ReadWebhooks()
	require.NoError(t, err)
	assert.Empty(t, view)
	assert.Empty(t, svc.ResolveTargets(""))

	raw := readConfigFile(t, svc)
	assert.Empty(t, raw["webhooks"])
	assert.NotContains(t, raw, "webhook")
}

func TestResolveTargetsReadsALegacySingleWebhook(t *testing.T) {
	svc := newService(t)

	require.NoError(t, os.WriteFile(svc.ConfigFilePath(), []byte(`{"webhook":"https://legacy.example/hook"}`), 0o644)) //nolint:gosec //fixture on purpose.

	// an installation that has not saved since the move still delivers.
	targets := svc.ResolveTargets("")
	require.Len(t, targets, 1)
	assert.Equal(t, "https://legacy.example/hook", targets[0].URL)
	assert.Equal(t, "legacy.example", targets[0].Name)
}

func TestResolveTargetsIgnoresABrokenConfigFile(t *testing.T) {
	svc := newService(t)

	require.NoError(t, os.WriteFile(svc.ConfigFilePath(), []byte("not json"), 0o644)) //nolint:gosec //broken fixture on purpose.

	// the daily report must still be generated; only the delivery is skipped.
	assert.Empty(t, svc.ResolveTargets(""))

	_, err := svc.ReadFileConfigView()
	require.Error(t, err)
}

func readConfigFile(t *testing.T, svc *service.Service) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(svc.ConfigFilePath())
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))

	return parsed
}
