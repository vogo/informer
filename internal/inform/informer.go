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

package inform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/configstore"
	"github.com/vogo/informer/internal/date"
	"github.com/vogo/informer/internal/ding"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/lark"
	"github.com/vogo/informer/internal/resultprocess"
	"github.com/vogo/informer/internal/soup"
)

// ConfigFileName is the informer configuration file inside the active data directory.
const ConfigFileName = "informer.json"

var (
	errUnsupportedBot = errors.New("unsupported bot address")
	errDeliveryFailed = errors.New("deliver the daily report failed for every bot")
)

const (
	// DataDirName is the directory holding the generated daily markdown files.
	DataDirName = "data"

	// DayLayout is the date layout of one daily markdown file, without the extension.
	DayLayout = "2006-01-02"

	// YearLayout is the layout of the year directory holding the daily files of one year.
	YearLayout = "2006"

	// DailyFileExt is the extension of a daily markdown file.
	DailyFileExt = ".md"
)

// DailyDirPath is the directory holding the daily markdown files of one year.
func DailyDirPath(homeDir string, day time.Time) string {
	return filepath.Join(homeDir, DataDirName, day.Format(YearLayout))
}

// DailyFilePath is the markdown file one inform run writes for the given day.
// It is the single definition of the daily layout: the writer here and every
// reader - the daily browser and the history index rebuild - share it.
func DailyFilePath(homeDir string, day time.Time) string {
	return filepath.Join(DailyDirPath(homeDir, day), day.Format(DayLayout)+DailyFileExt)
}

// Config is the layout of informer.json.
type Config struct {
	ResultProcessing *resultprocess.Config `json:"result_processing,omitempty"`

	Feed *feed.Config `json:"feed"`

	// Agent is the shared configuration of every agent source. A nil value runs
	// the agent with its defaults and the machine's own credentials.
	Agent *agent.Config `json:"agent"`

	// Webhooks are the bot delivery targets of every run. They are plain endpoints
	// rather than credentials, so they live in the shareable configuration file.
	Webhooks []Webhook `json:"webhooks,omitempty"`

	// HTTPProxy is the optional HTTP(S) proxy for URL fetches and agent runs.
	HTTPProxy string `json:"http_proxy,omitempty"`
}

// Webhook is one bot delivery target of informer.json.
type Webhook struct {
	// Name labels the target in the per target delivery result. An empty name is
	// rendered from the address host, so a target list stays readable when the
	// user only pastes addresses.
	Name string `json:"name,omitempty"`

	// URL is the bot webhook address.
	URL string `json:"url"`

	// Enabled takes part in every run when it is true or absent. Only an explicit
	// false pauses a target, which keeps hand written files short.
	Enabled *bool `json:"enabled,omitempty"`
}

// IsEnabled reports whether the target takes part in a run.
func (w Webhook) IsEnabled() bool {
	return w.Enabled == nil || *w.Enabled
}

// Target is one bot an inform run delivers to.
type Target struct {
	// Name labels the target in the per target delivery result.
	Name string

	// URL is the bot webhook address.
	URL string
}

// FeedProvider supplies recommended articles for a report. The caller binds it
// to the active data directory; the report workflow never opens a database.
type FeedProvider interface {
	AddFeeds(buf io.StringWriter, config *feed.Config, agentConfig *agent.Config) []*feed.Article
}

// Options describes one inform run.
type Options struct {
	ResultProcessing *resultprocess.Config

	// HomeDir is the active data directory holding informer.json and data/<year>.
	HomeDir string

	// Targets are the bots the message is delivered to; no target skips the
	// notification step and only writes the daily file.
	Targets []Target

	// FeedConfig is the effective feed configuration; a nil value skips feeds.
	FeedConfig *feed.Config

	// AgentConfig backs the agent sources of this run; a nil value runs them
	// with the agent defaults and the machine's own credentials.
	AgentConfig *agent.Config
}

// Result is the outcome of one inform run.
type Result struct {
	// Skipped is a successful AI decision to send nothing.
	Skipped              bool
	SkipReason           string
	ProcessingRecordPath string

	// Content is the generated message body.
	Content string

	// Articles are the feed articles selected for this run.
	Articles []*feed.Article

	// ContentFilePath is the daily markdown file the content was written to.
	ContentFilePath string

	// Notified reports whether at least one bot accepted the message.
	// It is false when no target was configured, and when every target failed.
	Notified bool

	// Deliveries is the outcome of every target, in the configured order, so a
	// partial failure can be reported target by target.
	Deliveries []Delivery
}

// Delivery is the outcome of delivering the message to one target.
type Delivery struct {
	// Name and URL identify the target the outcome belongs to.
	Name string
	URL  string

	// Success reports whether the bot accepted the message.
	Success bool

	// Error is the failure reason, empty on success.
	Error string
}

// Run builds the daily content, stores it and delivers it to every configured bot.
// A total delivery failure is returned as an error after the content has been written,
// so the caller can decide not to record a delivery that never happened.
func Run(opts *Options, feeds FeedProvider) (*Result, error) {
	validationErr := resultprocess.Validate(opts.ResultProcessing)
	if validationErr != nil {
		return nil, validationErr
	}
	today := time.Now()

	dataPath := DailyDirPath(opts.HomeDir, today)
	err := os.MkdirAll(dataPath, os.ModePerm)
	if err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("create data directory %q: %w", dataPath, err)
	}

	todayContentFilePath := DailyFilePath(opts.HomeDir, today)

	content, articles := initialContent(opts, feeds)

	result := &Result{
		Content:         content,
		Articles:        articles,
		ContentFilePath: todayContentFilePath,
	}

	if opts.ResultProcessing != nil && opts.ResultProcessing.Enabled {
		// Until processing succeeds there is no final content or selected article list.
		result.Content, result.ContentFilePath, result.Articles = "", "", nil
		report, recordPath, err := processFinal(context.Background(), opts, content, articles, today)
		result.ProcessingRecordPath = recordPath
		if err != nil {
			return result, err
		}
		if report.Action == "skip" {
			result.Skipped, result.SkipReason = true, report.Summary
			logger.Infof("final processing skipped this run: %s", report.Summary)

			return result, nil
		}
		content = report.Content
		result.Content, result.Articles = content, report.Selected(articles)
	}
	logger.Info(content)
	// A local-only run is delivered only if the final file was actually stored.
	writeErr := configstore.WriteAtomic(todayContentFilePath, []byte(content), configstore.PermConfig)
	if writeErr != nil {
		result.ContentFilePath = ""

		return result, fmt.Errorf("write daily report: %w", writeErr)
	}
	result.ContentFilePath = todayContentFilePath

	result.Deliveries = deliverAll(opts.Targets, content, today.Weekday())
	result.Notified = anyDelivered(result.Deliveries)

	// the message reaches every target in one run, so only a total failure is an
	// error: one bot accepting it is enough for the articles to count as informed.
	if len(result.Deliveries) > 0 && !result.Notified {
		return result, deliveryFailure(result.Deliveries)
	}

	return result, nil
}

// initialContent collects the date, daily sentence and selected feed articles.
func initialContent(opts *Options, feeds FeedProvider) (string, []*feed.Article) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString(date.GetDateInfo())

	if dailySoup := soup.GetDailySoup(); dailySoup != "" {
		buf.WriteString(dailySoup)
		buf.WriteByte('\n')
		buf.WriteByte('\n')
	}

	var articles []*feed.Article
	if opts.FeedConfig != nil {
		articles = feeds.AddFeeds(buf, opts.FeedConfig, opts.AgentConfig)
	}

	return buf.String(), articles
}

// deliverAll delivers the content to every target at once.
//
// The sends run concurrently because each one blocks for its own client deadline:
// delivering to N bots one after another would let a single silent endpoint hold
// the whole run - and the caller's run guard - for N timeouts.
func deliverAll(targets []Target, content string, weekday time.Weekday) []Delivery {
	if len(targets) == 0 {
		return nil
	}

	deliveries := make([]Delivery, len(targets))

	var wait sync.WaitGroup

	for index, target := range targets {
		wait.Go(func() {
			deliveries[index] = deliver(target, content, weekday)
		})
	}

	wait.Wait()

	return deliveries
}

// deliver delivers the content to one target and records its own outcome.
func deliver(target Target, content string, weekday time.Weekday) Delivery {
	delivery := Delivery{Name: target.Name, URL: target.URL}

	var err error

	switch {
	case strings.Contains(target.URL, lark.Host):
		err = lark.Lark(target.URL, content)
	case strings.Contains(target.URL, ding.Host):
		err = ding.Ding(target.URL, content, "", weekday)
	default:
		err = fmt.Errorf("%w, only %s and %s are recognized", errUnsupportedBot, ding.Host, lark.Host)
	}

	if err != nil {
		delivery.Error = err.Error()
	} else {
		delivery.Success = true
	}

	return delivery
}

// anyDelivered reports whether at least one target accepted the message.
func anyDelivered(deliveries []Delivery) bool {
	for _, delivery := range deliveries {
		if delivery.Success {
			return true
		}
	}

	return false
}

// deliveryFailure summarizes the failure of every target in one error, so a run
// that reached nobody still says why for each bot.
func deliveryFailure(deliveries []Delivery) error {
	details := make([]string, 0, len(deliveries))

	for _, delivery := range deliveries {
		name := delivery.Name
		if name == "" {
			name = delivery.URL
		}

		details = append(details, fmt.Sprintf("%s: %s", name, delivery.Error))
	}

	return fmt.Errorf("%w: %s", errDeliveryFailed, strings.Join(details, "; "))
}
