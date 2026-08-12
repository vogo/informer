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
	"time"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/inform"
)

// TriggerInform runs one full inform cycle: fetch, deduplicate, score, apply the
// same site limit, build the message, store the daily file and deliver it to the bot
// at urlAddr. It is the single entry point of the scheduled CLI notification.
//
// An empty urlAddr falls back to the webhook stored in informer.json, so a run
// started without an argument still delivers what the desktop app configured.
//
// The inform timestamp of the selected articles is written only after the
// notification step actually succeeded, so a failed delivery leaves InformedAt empty.
func (s *Service) TriggerInform(urlAddr string) (*inform.Result, error) {
	fileConfig, err := s.ReadFileConfig()
	if err != nil {
		return nil, err
	}

	result, err := inform.Run(&inform.Options{
		HomeDir:     s.homeDir,
		URLAddr:     s.ResolveWebhook(urlAddr),
		FeedConfig:  s.EffectiveFeedConfig(fileConfig),
		AgentConfig: s.EffectiveAgentConfig(fileConfig),
	})
	if err != nil {
		return result, err
	}

	s.recordInformed(result)

	return result, nil
}

// recordInformed stamps the delivery time on the articles of a completed run.
// A run without a bot webhook still counts as delivered: the daily file was written
// and nothing failed. A run whose notification failed never reaches this point.
func (s *Service) recordInformed(result *inform.Result) {
	if result == nil || len(result.Articles) == 0 {
		return
	}

	if err := feed.MarkArticlesInformed(result.Articles, time.Now().Unix()); err != nil {
		logger.Warnf("record inform timestamp failed: %v", err)
	}
}
