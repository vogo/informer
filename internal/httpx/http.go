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
	"bytes"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

const (
	DefaultMaxIdleConns        = 32
	DefaultMaxIdleConnsPerHost = 8
	DefaultMaxConnsPerHost     = 64
	DefaultIdleConnTimeout     = time.Second * 8

	DefaultRequestTimeout = time.Second * 60
)

// nolint:gochecknoglobals //ignore this.
var jar, _ = cookiejar.New(nil)

// HTTPClient the default http client.
// nolint:exhaustivestruct,gochecknoglobals // ignore this
var HTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        DefaultMaxIdleConns,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		MaxConnsPerHost:     DefaultMaxConnsPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
	},
	Timeout: DefaultRequestTimeout,
	Jar:     jar,
}

// nolint:gochecknoglobals //ignore this.
var defaultHTTPHeaders = map[string]string{
	"accept":          "*/*",
	"accept-language": "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7",
	"user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.51 Safari/537.36",
	"mode": "cors",
}

func GetLinkData(link string) ([]byte, error) {
	httpReq, err := http.NewRequest(http.MethodGet, link, bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}

	for k, v := range defaultHTTPHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
