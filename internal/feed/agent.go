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

package feed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vogo/logger"
	"github.com/vogo/vogo/vnet/vurl"

	"github.com/vogo/informer/internal/agent"
)

// ErrAgentPromptEmpty marks an agent source that has no instruction to run.
var ErrAgentPromptEmpty = errors.New("agent source has no prompt")

func agentParseFeed(config *Config, source *Source, agentConfig *agent.Config, _ int64) {
	logger.Info("agent parse feed: ", source.Title)

	articles, err := AgentParse(source, agentConfig)
	if err != nil {
		logger.Infof("agent parse feed error! source: %s, error: %v", source.Title, err)

		updateSourceError(source, err)

		return
	}

	updateSourceNormal(source)

	saveParsedArticles(config, source, articles)
}

// AgentParse asks the configured agent for the articles of an agent source.
//
// The source only carries the instruction; the json output contract and the item
// limit are added here, so what the user typed stays exactly what they meant.
// The source may override the provider of the shared agent configuration, which
// is what lets one subscription run on a different agent than the rest.
func AgentParse(source *Source, agentConfig *agent.Config) ([]*Article, error) {
	prompt := strings.TrimSpace(source.AgentPrompt)
	if prompt == "" {
		return nil, ErrAgentPromptEmpty
	}

	runConfig := agentConfig.Normalized()
	if provider := strings.TrimSpace(source.AgentProvider); provider != "" {
		runConfig.Provider = provider
	}

	result, err := agent.Run(context.Background(), runConfig, prompt, source.MaxFetchNum)
	if err != nil {
		return nil, fmt.Errorf("agent source %q: %w", source.Title, err)
	}

	now := time.Now().Unix()

	articles := make([]*Article, 0, len(result.Items))

	for _, item := range result.Items {
		link := item.URL
		if source.Redirect {
			link = vurl.RedirectURL(link)
		}

		articles = append(articles, &Article{
			URL:       link,
			Title:     item.Title,
			Timestamp: now,
			Weight:    source.Weight,
			SourceID:  source.ID,
		})
	}

	logger.Infof("agent parse, source: %s, articles: %d", source.Title, len(articles))

	return articles, nil
}
