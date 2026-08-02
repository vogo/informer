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

package main

import (
	"errors"

	"github.com/vogo/informer/internal/inform"
)

// InformResultDTO is what one manual push did. A failed delivery is data, not a
// binding error: the daily file may already be written when the bot rejects the
// message, and the page then shows both facts at once.
type InformResultDTO struct {
	// Success reports whether the whole run - fetch, daily file and delivery -
	// finished without an error.
	Success bool `json:"success"`

	// Notified reports whether a bot accepted the message. False together with
	// Success means no webhook is configured, so only the daily file was written.
	Notified bool `json:"notified"`

	// ArticleCount is how many articles the run selected into the message.
	ArticleCount int `json:"articleCount"`

	// ContentFilePath is where the daily markdown landed, empty when the run
	// failed before building it.
	ContentFilePath string `json:"contentFilePath"`

	// ErrorInfo carries the failure reason when Success is false, empty otherwise.
	ErrorInfo string `json:"errorInfo"`
}

// TriggerNow runs one full inform cycle at once: fetch every enabled subscription,
// build the daily message, store the markdown and deliver it to the configured bot.
//
// The binding fails only on a broken startup or a run already in flight; every
// pipeline failure arrives inside the result, so the page can still show what the
// run managed to write.
func (a *App) TriggerNow() (*InformResultDTO, error) {
	err := a.ready()
	if err != nil {
		return nil, err
	}

	result, err := a.runInform()
	if errors.Is(err, ErrInformRunning) {
		return nil, err
	}

	return toInformResultDTO(result, err), nil
}

func toInformResultDTO(result *inform.Result, err error) *InformResultDTO {
	dto := &InformResultDTO{}

	if result != nil {
		dto.Notified = result.Notified
		dto.ArticleCount = len(result.Articles)
		dto.ContentFilePath = result.ContentFilePath
	}

	if err != nil {
		dto.ErrorInfo = err.Error()

		return dto
	}

	dto.Success = true

	return dto
}
