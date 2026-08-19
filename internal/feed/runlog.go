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
	"fmt"
	"net/http"
	"time"

	"github.com/vogo/informer/internal/agent"
	"github.com/vogo/informer/internal/httpx"
	"github.com/vogo/informer/internal/runlog"
)

// maxLoggedBodyRunes bounds how much of a fetched document is quoted into the
// log. A page that matched nothing is diagnosed from its opening, and a 520px
// panel is not a terminal.
const maxLoggedBodyRunes = 2000

// millisecond is the resolution durations are reported at.
const millisecond = time.Millisecond

// httpTrace turns a sink into the callback httpx reports exchanges to. A parser
// that hands the fetch to a library - gofeed reads the url itself - has no other
// place to see the status code from.
func httpTrace(sink runlog.Sink) func(*httpx.FetchStats) {
	if sink == nil {
		return nil
	}

	return func(stats *httpx.FetchStats) {
		logFetchStats(sink, stats)
	}
}

// logFetchStats records one finished http exchange in one line.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func logFetchStats(sink runlog.Sink, stats *httpx.FetchStats) {
	line := fmt.Sprintf("GET %s → %d %s，耗时 %s",
		stats.URL, stats.StatusCode, byteSize(stats.Bytes), stats.Duration.Round(millisecond))

	// a refused page still comes back as bytes and would otherwise be parsed as
	// if it were the source; saying so is usually the whole diagnosis.
	if stats.StatusCode < http.StatusOK || stats.StatusCode >= http.StatusBadRequest {
		runlog.Warnf(sink, "%s", line)

		return
	}

	runlog.Infof(sink, "%s", line)
}

// agentObserver adapts a sink to what the agent package reports through. The
// note levels of that package are the same strings this one records, which is
// what keeps the adapter a single line of mapping.
func agentObserver(sink runlog.Sink) agent.Observer {
	if sink == nil {
		return nil
	}

	return agent.ObserverFunc(func(level, text string) {
		runlog.Log(sink, level, text)
	})
}

// byteSize renders a body size the way a person reads it. A size of -1 means it
// was never counted, which is how a traced client reports a response whose
// length the server did not declare.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func byteSize(bytes int) string {
	const unit = 1024

	switch {
	case bytes < 0:
		return "大小未知"
	case bytes < unit:
		return fmt.Sprintf("%dB", bytes)
	case bytes < unit*unit:
		return fmt.Sprintf("%.1fKB", float64(bytes)/unit)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(unit*unit))
	}
}
