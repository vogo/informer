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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/resultprocess"
)

const resultProcessingKey = "result_processing"

// SaveResultProcessingConfig stores only the final processing section.
func (s *Service) SaveResultProcessingConfig(config *resultprocess.Config) error {
	if config == nil {
		return fmt.Errorf("%w: result processing config is nil", ErrInvalidArgument)
	}
	validationErr := resultprocess.Validate(config)
	if validationErr != nil {
		return fmt.Errorf("%w: %w", ErrInvalidArgument, validationErr)
	}

	return s.saveConfigSection(resultProcessingKey, config)
}

// ResultProcessingPreview contains the input, output and decisions of a read-only trial.
type ResultProcessingPreview struct {
	Original   string                `json:"original"`
	Candidates []*feed.Article       `json:"candidates"`
	Report     *resultprocess.Report `json:"report"`
}

// PreviewResultProcessing processes existing candidates only. It neither fetches
// nor saves a daily report nor modifies any source or article state.
//
//nolint:contextcheck //command discovery is deliberately disabled in this read-only preview.
func (s *Service) PreviewResultProcessing(ctx context.Context, prompt string) (*ResultProcessingPreview, error) {
	config := &resultprocess.Config{Enabled: true, Prompt: prompt}
	validationErr := resultprocess.Validate(config)
	if validationErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArgument, validationErr)
	}
	fileConfig, err := s.ReadFileConfig()
	if err != nil {
		return nil, err
	}
	feedConfig := s.EffectiveFeedConfig(fileConfig)
	if feedConfig != nil {
		validationErr = ValidateFeedConfig(feedConfig)
		if validationErr != nil {
			return nil, validationErr
		}
	}
	articles, err := feed.SelectArticles(feedConfig)
	if err != nil {
		return nil, err
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("%w: no unnotified candidate articles in the article library", ErrNotFound)
	}
	day := time.Now()
	var original strings.Builder
	fmt.Fprintf(&original, "# %s\n\n", day.Format("2006-01-02"))
	for _, article := range articles {
		fmt.Fprintf(&original, "- %s, %s\n", article.Title, article.URL)
	}
	agentConfig := s.effectiveAgentConfig(fileConfig, false)
	// Progress reaches the system log; preview has no stored output or delivery.
	report, err := resultprocess.Process(ctx, config, agentConfig, original.String(), articles, day, agent.ObserverFunc(processingNote))
	if err != nil {
		return nil, err
	}

	return &ResultProcessingPreview{Original: original.String(), Candidates: articles, Report: report}, nil
}

func processingNote(_, text string) { logger.Infof("final processing preview: %s", text) }
