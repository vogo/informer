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

//nolint:gosmopolitan //Exercise Chinese group names through JSON and settings validation.
package service_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/wechat"
)

func TestWeChatConfigRoundTripPreservesWebhooks(t *testing.T) {
	svc := newService(t)
	require.NoError(t, svc.SaveWebhooks([]inform.Webhook{{Name: "robot", URL: "https://example.com/hook"}}))
	require.NoError(t, svc.SaveWeChat(wechat.Config{Enabled: true, GroupName: " 技术群 "}))
	cfg, err := svc.ReadFileConfig()
	require.NoError(t, err)
	require.Equal(t, "技术群", cfg.WeChat.GroupName)
	require.True(t, cfg.WeChat.Enabled)
	require.Len(t, cfg.Webhooks, 1)
	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	require.Equal(t, *cfg.WeChat, *view.WeChat)
	require.NotContains(t, view.PreservedKeys, "wechat")
	before, err := os.ReadFile(svc.ConfigFilePath())
	require.NoError(t, err)
	require.Error(t, svc.SaveWeChat(wechat.Config{Enabled: true}))
	after, err := os.ReadFile(svc.ConfigFilePath())
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// A malformed hand-written WeChat target fails before touching the real desktop,
// while the independent webhook still succeeds. Explicit CLI URLs override both.
func TestWeChatFailureDoesNotBlockWebhookAndCLIOverride(t *testing.T) {
	server := newContentServer(t)
	svc := newService(t)
	require.NoError(t, svc.CreateSource(feedSource(server)))
	cfg, err := svc.ReadFileConfig()
	require.NoError(t, err)
	cfg.Webhooks = []inform.Webhook{{Name: "robot", URL: server.URL + "/feishu.cn/ok"}}
	cfg.WeChat = &wechat.Config{Enabled: true, GroupName: ""}
	writeConfigFile(t, svc.HomeDir(), cfg)
	result, err := svc.TriggerInform("")
	require.NoError(t, err)
	require.True(t, result.Notified)
	require.Len(t, result.Deliveries, 2)
	require.True(t, result.Deliveries[0].Success)
	require.False(t, result.Deliveries[1].Success)
	require.Contains(t, result.Deliveries[1].Error, "群名")
	result, err = svc.TriggerInform(server.URL + "/feishu.cn/ok")
	require.NoError(t, err)
	require.Len(t, result.Deliveries, 1)
	require.True(t, result.Deliveries[0].Success)
}
