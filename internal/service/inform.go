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
// same site limit, build the message, store the daily file and deliver it to every
// configured bot. It is the single entry point of the scheduled CLI notification.
//
// An empty urlAddr falls back to the targets stored in informer.json, so a run
// started without an argument still delivers what the desktop app configured; an
// explicit address replaces the stored list for this run only.
//
// The inform timestamp of the selected articles is written only after at least one
// bot accepted the message, so a run that reached nobody leaves InformedAt empty.
func (s *Service) TriggerInform(urlAddr string) (*inform.Result, error) {
	s.ApplyHTTPProxy()

	fileConfig, err := s.ReadFileConfig()
	if err != nil {
		return nil, err
	}

	result, err := inform.Run(&inform.Options{
		HomeDir:          s.homeDir,
		Targets:          s.ResolveTargets(urlAddr),
		FeedConfig:       s.EffectiveFeedConfig(fileConfig),
		AgentConfig:      s.EffectiveAgentConfig(fileConfig),
		ResultProcessing: fileConfig.ResultProcessing,
	})
	if err != nil {
		return result, err
	}

	s.recordInformed(result)

	return result, nil
}

// recordInformed stamps the delivery time on the articles of a completed run.
// A run without a target still counts as delivered: the daily file was written and
// nothing failed. A run where at least one bot accepted the message counts too, even
// if another one failed - the timestamp is per article, not per target, so a failed
// target is not retried on its own later.
func (s *Service) recordInformed(result *inform.Result) {
	if result == nil || result.Skipped || len(result.Articles) == 0 {
		return
	}

	if len(result.Deliveries) > 0 && !result.Notified {
		return
	}

	err := feed.MarkArticlesInformed(result.Articles, time.Now().Unix())
	if err != nil {
		logger.Warnf("record inform timestamp failed: %v", err)
	}
}
