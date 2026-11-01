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

//nolint:gosmopolitan //The fixed test message is shown verbatim in the Chinese settings UI.
package main

import (
	"time"

	"github.com/vogo/informer/internal/wechat"
)

// WeChatConfigDTO configures one local desktop group target.
type WeChatConfigDTO struct {
	Enabled   bool   `json:"enabled"`
	GroupName string `json:"groupName"`
}

type WeChatOutcomeDTO struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (a *App) SaveWeChat(req WeChatConfigDTO) error {
	err := a.ready()
	if err != nil {
		return err
	}

	return a.svc.SaveWeChat(wechat.Config{Enabled: req.Enabled, GroupName: req.GroupName})
}

func (a *App) CheckWeChat() (*WeChatOutcomeDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	return wechatOutcome(wechat.Check())
}

func (a *App) LocateWeChat(group string) (*WeChatOutcomeDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	return wechatOutcome(wechat.Locate(group))
}

// TestWeChat sends only the fixed test text shown in settings, never a report.
func (a *App) TestWeChat(group string) (*WeChatOutcomeDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	return wechatOutcome(wechat.Send(group, "informer 微信推送测试\n时间："+time.Now().Format("2006-01-02 15:04:05")))
}

func (a *App) ResolveWeChatPending(group string) error {
	err := a.ready()
	if err != nil {
		return err
	}

	return wechat.ResolvePending(group)
}

func wechatOutcome(result *wechat.Outcome, err error) (*WeChatOutcomeDTO, error) {
	if err != nil {
		return nil, err
	}

	return &WeChatOutcomeDTO{Status: result.Status, Message: result.Message}, nil
}
