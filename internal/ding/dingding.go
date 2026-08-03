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

package ding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/httpx"
)

const Host = "dingtalk.com"

// ErrDingResponse marks a dingtalk bot answering with a non successful status.
var ErrDingResponse = errors.New("ding response error")

// client delivers the webhook request. It is the shared httpx client whose 60s
// timeout turns a hung connection into a delivery failure, so one frozen bot
// can never occupy the inform run forever. It is a variable so tests can
// shorten the deadline without touching the production default.
var client = httpx.HTTPClient

// SetClientForTest swaps the delivery client and returns a restore function.
// It is the seam the timeout tests use to fail a frozen webhook in
// milliseconds instead of waiting out the full production deadline.
func SetClientForTest(c *http.Client) func() {
	previous := client
	client = c

	return func() { client = previous }
}

type MsgText struct {
	Content string `json:"content"`
}

type MsgAt struct {
	AtMobiles []string `json:"atMobiles"`
}

type MsgBody struct {
	MsgType string  `json:"msgtype"`
	Text    MsgText `json:"text"`
	At      MsgAt   `json:"at"`
}

// Ding sends the content to a dingtalk bot. It reports delivery failures to the caller
// instead of killing the process, so the caller can tell a delivered notification
// from a failed one.
func Ding(url, content, user string, weekday time.Weekday) error {
	msg := &MsgBody{
		MsgType: "text",
		Text: MsgText{
			Content: content,
		},
	}

	if user != "" && weekday != time.Sunday && weekday != time.Saturday {
		msg.At = MsgAt{AtMobiles: []string{user}}
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal ding message: %w", err)
	}

	logger.Infof("ding url: %s", url)
	logger.Infof("ding data: %s", data)

	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post ding message: %w", err)
	}

	defer resp.Body.Close()

	logger.Infof("ding response: %v", resp)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s", ErrDingResponse, resp.Status)
	}

	return nil
}
