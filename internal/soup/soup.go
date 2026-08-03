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

package soup

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/httpx"
)

// dailyURL is the endpoint of the daily sentence. It is a variable so tests
// can aim the fetch at a local server instead of the public one.
var dailyURL = "http://open.iciba.com/dsapi/"

// client fetches the daily sentence. It is the shared httpx client whose 60s
// timeout turns a hung connection into the empty fallback, so a frozen
// endpoint can never occupy the inform run forever. It is a variable so tests
// can shorten the deadline without touching the production default.
var client = httpx.HTTPClient

// SetClientForTest swaps the fetch client and returns a restore function.
// It is the seam the timeout tests use to degrade a frozen endpoint in
// milliseconds instead of waiting out the full production deadline.
func SetClientForTest(c *http.Client) func() {
	previous := client
	client = c

	return func() { client = previous }
}

// SetURLForTest swaps the daily sentence endpoint and returns a restore
// function, so tests can hermetically exercise the fetch and its fallback.
func SetURLForTest(url string) func() {
	previous := dailyURL
	dailyURL = url

	return func() { dailyURL = previous }
}

func GetDailySoup() string {
	resp, err := client.Get(dailyURL)
	if err != nil {
		logger.Infof("err: %v", err)

		return ""
	}
	defer resp.Body.Close()

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Infof("err: %v", err)

		return ""
	}

	if resp.StatusCode != http.StatusOK {
		logger.Infof("err: %d, %s", resp.StatusCode, buf)

		return ""
	}

	data := struct {
		Content string `json:"content"`
	}{}

	if err = json.Unmarshal(buf, &data); err != nil {
		logger.Infof("err: %v", err)

		return ""
	}

	return data.Content
}
