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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/date"
	"github.com/vogo/informer/internal/ding"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/lark"
	"github.com/vogo/informer/internal/soup"
)

// ConfigFileName is the informer configuration file inside the active data directory.
const ConfigFileName = "informer.json"

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
	Feed *feed.Config `json:"feed"`
}

// Options describes one inform run. The feed database is expected to be
// initialised by the caller, so this package never opens it itself.
type Options struct {
	// HomeDir is the active data directory holding informer.json and data/<year>.
	HomeDir string

	// URLAddr is the bot webhook; an empty value skips the notification step.
	URLAddr string

	// FeedConfig is the effective feed configuration; a nil value skips feeds.
	FeedConfig *feed.Config
}

// Result is the outcome of one inform run.
type Result struct {
	// Content is the generated message body.
	Content string

	// Articles are the feed articles selected for this run.
	Articles []*feed.Article

	// ContentFilePath is the daily markdown file the content was written to.
	ContentFilePath string

	// Notified reports whether a bot actually accepted the message.
	// It is false when no webhook was configured.
	Notified bool
}

// Run builds the daily content, stores it and delivers it to the configured bot.
// A notification failure is returned as an error after the content has been written,
// so the caller can decide not to record a delivery that never happened.
func Run(opts *Options) (*Result, error) {
	today := time.Now()

	dataPath := DailyDirPath(opts.HomeDir, today)
	if err := os.MkdirAll(dataPath, os.ModePerm); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("create data directory %q: %w", dataPath, err)
	}

	todayContentFilePath := DailyFilePath(opts.HomeDir, today)

	buf := bytes.NewBuffer(nil)
	buf.WriteString(date.GetDateInfo())

	if dailySoup := soup.GetDailySoup(); dailySoup != "" {
		buf.WriteString(dailySoup)
		buf.WriteByte('\n')
		buf.WriteByte('\n')
	}

	weekday := today.Weekday()

	var articles []*feed.Article
	if opts.FeedConfig != nil {
		articles = feed.AddFeeds(buf, opts.FeedConfig)
	}

	content := buf.String()
	logger.Info(content)

	result := &Result{
		Content:         content,
		Articles:        articles,
		ContentFilePath: todayContentFilePath,
	}

	if err := os.WriteFile(todayContentFilePath, []byte(content), os.ModePerm); err != nil {
		logger.Warnf("write today content to file failed: %v", err)
	}

	notified, err := notify(opts.URLAddr, content, weekday)
	result.Notified = notified

	if err != nil {
		return result, err
	}

	return result, nil
}

// notify delivers the content and reports whether a bot accepted it.
func notify(urlAddr, content string, weekday time.Weekday) (bool, error) {
	if urlAddr == "" {
		return false, nil
	}

	switch {
	case strings.Contains(urlAddr, lark.Host):
		if err := lark.Lark(urlAddr, content); err != nil {
			return false, err
		}

		return true, nil
	case strings.Contains(urlAddr, ding.Host):
		if err := ding.Ding(urlAddr, content, "", weekday); err != nil {
			return false, err
		}

		return true, nil
	default:
		// an unknown address is not a bot webhook, the same as before.
		return false, nil
	}
}
