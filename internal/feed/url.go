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
	"net/url"
	"strings"
	"time"

	"github.com/vogo/logger"
	"github.com/vogo/vogo/vos"

	"github.com/vogo/informer/internal/httpx"
	"github.com/vogo/informer/internal/runlog"
)

func FormatURL(link string) (string, bool) {
	if isURLContainsInvalidChars(link) {
		return "", false
	}

	u, err := url.Parse(link)
	if err != nil {
		logger.Info("format url error!", err)

		return "", false
	}

	params := u.Query()
	for key := range params {
		if strings.HasPrefix(key, "utm_") {
			params.Del(key)
		}
	}

	u.RawQuery = params.Encode()

	return u.String(), true
}

func isURLContainsInvalidChars(link string) bool {
	return strings.Contains(link, "%22") ||
		strings.Contains(link, "%20") ||
		strings.Contains(link, "%3C")
}

// GetHostFromURL get host from url,
// host is www.blog.com if url is http://www.blog.com/page.html.
func GetHostFromURL(host string) string {
	hostIndex := strings.Index(host, "//")
	if hostIndex > 0 {
		host = host[hostIndex+2:]
	}

	hostIndex = strings.Index(host, "/")
	if hostIndex > 0 {
		host = host[:hostIndex]
	}

	return host
}

// GetHostPrefix get host prefix from url,
// prefix is http://www.blog.com if url is http://www.blog.com/page.html.
func GetHostPrefix(link string) string {
	protocolIndex := strings.Index(link, "//")
	if protocolIndex < 0 {
		protocolIndex = 0
	}

	hostIndex := strings.Index(link[protocolIndex+2:], "/")
	if hostIndex > 0 {
		return link[:protocolIndex+2+hostIndex]
	}

	return link
}

// readURLData fetches the document a source parses, through the shell command it
// carries or through plain http, and records what the exchange looked like.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func readURLData(source *Source, sink runlog.Sink) ([]byte, error) {
	if source.CURL != "" {
		return readCURLData(source, sink)
	}

	started := time.Now()

	data, stats, err := httpx.GetLinkDataStats(source.URL)
	if err != nil {
		runlog.Errorf(sink, "GET %s 失败，耗时 %s：%v", source.URL, time.Since(started).Round(millisecond), err)

		return nil, err
	}

	logFetchStats(sink, stats)

	return data, nil
}

// readCURLData runs the source's own curl line.
//
// vos.ExecShell returns the combined output, so a failing curl writes its
// complaint into what would otherwise be the document; saying which one happened
// is the point of logging it separately.
//
//nolint:gosmopolitan //informer is a chinese product; the recorded lines speak the user's language.
func readCURLData(source *Source, sink runlog.Sink) ([]byte, error) {
	runlog.Infof(sink, "执行 curl：%s", source.CURL)

	started := time.Now()

	data, err := vos.ExecShell(source.CURL)
	elapsed := time.Since(started).Round(millisecond)

	if err != nil {
		runlog.Errorf(sink, "curl 失败，耗时 %s：%v，输出：%s",
			elapsed, err, runlog.Truncate(string(data), maxLoggedBodyRunes))

		return nil, err
	}

	runlog.Infof(sink, "curl 返回 %s，耗时 %s", byteSize(len(data)), elapsed)

	return data, nil
}

func adjustLink(hostPrefix, link string) string {
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		if link[0] != '/' {
			link = hostPrefix + "/" + link
		} else {
			link = hostPrefix + link
		}
	}

	return link
}
