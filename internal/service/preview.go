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

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/inform"
)

// Preview parses a stored source and returns its candidate articles.
//
// It reads the source and then performs network reads only: no source, article,
// health status, timestamp or configuration record is created or modified. The
// Enabled flag does not restrict a preview, and a parse failure is reported as an
// error without marking the source unhealthy.
func (s *Service) Preview(sourceID int64) ([]*feed.Article, error) {
	source, err := s.GetSource(sourceID)
	if err != nil {
		return nil, err
	}

	return s.PreviewSource(source)
}

// PreviewSource parses an in memory source without persisting anything at all,
// which lets a caller try out a source before it is stored.
func (s *Service) PreviewSource(source *feed.Source) ([]*feed.Article, error) {
	err := ValidateSource(source)
	if err != nil {
		return nil, err
	}

	articles, err := feed.ParseArticles(source, s.agentConfig())
	if err != nil {
		return nil, fmt.Errorf("preview source %d: %w", source.ID, err)
	}

	return articles, nil
}

// agentConfig resolves the agent configuration a fetch outside of a full inform
// run works with. A broken informer.json is not allowed to make a preview
// impossible, so the defaults stand in for a section that cannot be read.
func (s *Service) agentConfig() *agent.Config {
	stored, err := s.ReadAgentConfig()
	if err != nil {
		logger.Warnf("read agent config failed, using defaults: %v", err)

		stored = DefaultAgentConfig()
	}

	return s.EffectiveAgentConfig(&inform.Config{Agent: stored})
}
