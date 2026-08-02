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
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/service"
)

// windowsGOOS is the platform that does not model unix permission bits.
const windowsGOOS = "windows"

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

func TestSaveWebhookWritesASensitiveFile(t *testing.T) {
	svc := newService(t)

	view, err := svc.ReadSecretsView()
	require.NoError(t, err)
	assert.False(t, view.Exists)
	assert.False(t, view.WebhookConfigured)
	assert.Empty(t, view.WebhookMasked)

	const webhook = "https://oapi.dingtalk.com/robot/send?access_token=super-secret-token"

	require.NoError(t, svc.SaveWebhook(webhook))

	view, err = svc.ReadSecretsView()
	require.NoError(t, err)
	assert.True(t, view.Exists)
	assert.True(t, view.WebhookConfigured)
	assert.NotContains(t, view.WebhookMasked, "super-secret-token")
	assert.Contains(t, view.WebhookMasked, "oapi.dingtalk.co")

	assertSecretPermission(t, svc.SecretsFilePath())

	// the stored webhook is what an argument free inform run delivers to.
	assert.Equal(t, webhook, svc.ResolveWebhook(""))
	assert.Equal(t, webhook, svc.ResolveWebhook("   "))

	// an explicit command line address still wins, so crontab keeps its behavior.
	assert.Equal(t, "https://example.com/hook", svc.ResolveWebhook("https://example.com/hook"))
}

func TestSaveWebhookRewritesAnInsecureFileSecurely(t *testing.T) {
	if runtime.GOOS == windowsGOOS {
		t.Skip("windows does not model unix permission bits")
	}

	svc := newService(t)

	// deliberately world readable, to prove the save locks the file down again.
	require.NoError(t, os.WriteFile(svc.SecretsFilePath(), []byte(`{"webhook":"old"}`), 0o644)) //nolint:gosec //see above.
	require.NoError(t, svc.SaveWebhook("https://example.com/new"))

	assertSecretPermission(t, svc.SecretsFilePath())
	assert.Equal(t, "https://example.com/new", svc.ResolveWebhook(""))
}

func TestSaveWebhookClearsTheStoredValue(t *testing.T) {
	svc := newService(t)

	require.NoError(t, svc.SaveWebhook("https://example.com/hook"))
	require.NoError(t, svc.SaveWebhook(""))

	view, err := svc.ReadSecretsView()
	require.NoError(t, err)
	assert.True(t, view.Exists)
	assert.False(t, view.WebhookConfigured)
	assert.Empty(t, svc.ResolveWebhook(""))

	assertSecretPermission(t, svc.SecretsFilePath())
}

func TestResolveWebhookIgnoresABrokenSecretFile(t *testing.T) {
	svc := newService(t)

	require.NoError(t, os.WriteFile(svc.SecretsFilePath(), []byte("not json"), 0o600))

	// the daily report must still be generated; only the delivery is skipped.
	assert.Empty(t, svc.ResolveWebhook(""))

	_, err := svc.ReadSecretsView()
	require.Error(t, err)
}

func assertSecretPermission(t *testing.T, path string) {
	t.Helper()

	if runtime.GOOS == windowsGOOS {
		return
	}

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func readConfigFile(t *testing.T, svc *service.Service) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(svc.ConfigFilePath())
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))

	return parsed
}
