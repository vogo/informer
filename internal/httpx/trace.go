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

package httpx

import (
	"net/http"
	"time"
)

// headerContentType names the response header the stats report.
const headerContentType = "Content-Type"

// FetchStats is what one http exchange looked like from the outside.
//
// It carries no body: the point is to be cheap enough to log for every fetch,
// and to answer "did the server even say yes" without reprinting the page.
type FetchStats struct {
	// URL is the address that was read.
	URL string

	// StatusCode is what the server answered, zero when the request never
	// reached one.
	StatusCode int

	// Bytes is the size of the decoded body. It is -1 when the size was not
	// counted, which is how a traced client reports a response whose length the
	// server did not declare.
	Bytes int

	// ContentType is the declared type of the body, empty when there was none.
	ContentType string

	// Duration is how long the exchange took, request through last byte.
	Duration time.Duration
}

// tracingTransport reports every exchange it carries and otherwise stays out of
// the way. It never reads the body: a parser downstream needs those bytes, and
// buffering a feed twice to count them is not worth the memory.
type tracingTransport struct {
	base  http.RoundTripper
	trace func(stats *FetchStats)
}

func (t *tracingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	started := time.Now()

	response, err := t.base.RoundTrip(request)

	stats := &FetchStats{URL: request.URL.String(), Bytes: -1, Duration: time.Since(started)}

	if response != nil {
		stats.StatusCode = response.StatusCode
		stats.ContentType = response.Header.Get(headerContentType)

		if response.ContentLength >= 0 {
			stats.Bytes = int(response.ContentLength)
		}
	}

	t.trace(stats)

	return response, err
}

// NewTracingClient returns a client that reports every exchange to trace and is
// in every other way the shared HTTPClient: same transport, so the configured
// proxy applies, and the same request timeout.
//
// It exists for the parsers that hand the fetch to a library - gofeed reads the
// url itself - where wrapping the client is the only seam left to see the
// status code through.
func NewTracingClient(trace func(stats *FetchStats)) *http.Client {
	if trace == nil {
		return HTTPClient
	}

	return &http.Client{
		Transport: &tracingTransport{base: HTTPClient.Transport, trace: trace},
		Timeout:   HTTPClient.Timeout,
	}
}
