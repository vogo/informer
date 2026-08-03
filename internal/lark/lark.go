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

package lark

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/httpx"
)

const Host = "feishu.cn"

// ErrLarkResponse marks a lark bot answering with a non successful status.
var ErrLarkResponse = errors.New("lark response error")

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

type Content struct {
	Text string `json:"text"`
}

type Message struct {
	Type    string   `json:"msg_type"`
	Content *Content `json:"content"`
}

// Lark sends the content to a lark bot. It reports delivery failures to the caller
// instead of killing the process, so the caller can tell a delivered notification
// from a failed one.
func Lark(url, content string) error {
	msg := &Message{
		Type: "text",
		Content: &Content{
			Text: content,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal lark message: %w", err)
	}

	logger.Infof("lark url: %s", url)
	logger.Infof("lark data: %s", data)

	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post lark message: %w", err)
	}
	defer resp.Body.Close()

	logger.Infof("lark response: %v", resp)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s", ErrLarkResponse, resp.Status)
	}

	return nil
}
