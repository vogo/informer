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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/configstore"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/resultprocess"
)

// processingRecord stays outside data/, which is scanned as delivered history.
// It is stored before invoking the agent so failures never lose their candidates.
type processingRecord struct {
	Date       time.Time             `json:"date"`
	Config     *resultprocess.Config `json:"config"`
	Original   string                `json:"original"`
	Candidates []*feed.Article       `json:"candidates"`
	Report     *resultprocess.Report `json:"report,omitempty"`
	Error      string                `json:"error,omitempty"`
}

func processFinal(ctx context.Context, opts *Options, original string, articles []*feed.Article, day time.Time) (*resultprocess.Report, string, error) {
	dir := filepath.Join(opts.HomeDir, "runs", day.Format(DayLayout))
	mkdirErr := os.MkdirAll(dir, 0o700)
	if mkdirErr != nil {
		return nil, "", fmt.Errorf("create processing records: %w", mkdirErr)
	}
	file, err := os.CreateTemp(dir, "processing-*.json")
	if err != nil {
		return nil, "", fmt.Errorf("create processing record: %w", err)
	}
	path := file.Name()
	closeErr := file.Close()
	if closeErr != nil {
		return nil, path, fmt.Errorf("close processing record: %w", closeErr)
	}
	record := &processingRecord{Date: day, Config: opts.ResultProcessing, Original: original, Candidates: articles}
	err = saveProcessingRecord(path, record)
	if err != nil {
		return nil, path, err
	}
	logger.Infof("final processing started: %d candidates, record %s", len(articles), path)
	report, processErr := resultprocess.Process(ctx, opts.ResultProcessing, opts.AgentConfig, original, articles, day,
		agent.ObserverFunc(func(level, text string) {
			if level == agent.NoteError || level == agent.NoteWarn {
				logger.Warnf("final processing: %s", text)
			} else {
				logger.Infof("final processing: %s", text)
			}
		}))
	record.Report = report
	if processErr != nil {
		record.Error = processErr.Error()
	}
	err = saveProcessingRecord(path, record)
	if err != nil {
		return report, path, errors.Join(processErr, err)
	}
	if processErr != nil {
		return report, path, fmt.Errorf("final processing failed (record %s): %w", path, processErr)
	}
	logger.Infof("final processing completed: %d selected, %d rejected", len(report.Articles), len(report.Rejected))

	return report, path, nil
}

func saveProcessingRecord(path string, record *processingRecord) error {
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode processing record: %w", err)
	}
	err = configstore.WriteAtomic(path, raw, configstore.PermConfig)
	if err != nil {
		return fmt.Errorf("write processing record: %w", err)
	}

	return nil
}
