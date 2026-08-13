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
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html/charset"
)

const (
	DefaultMaxIdleConns        = 32
	DefaultMaxIdleConnsPerHost = 8
	DefaultMaxConnsPerHost     = 64
	DefaultIdleConnTimeout     = time.Second * 8

	DefaultRequestTimeout = time.Second * 60
)

//nolint:gochecknoglobals //ignore this.
var jar, _ = cookiejar.New(nil)

// configuredProxy holds the optional HTTP(S) proxy URL for HTTPClient.
// It is read through Transport.Proxy so a settings change never races a
// request that is already reading the Proxy field.
//
//nolint:gochecknoglobals //shared client state.
var configuredProxy atomic.Pointer[url.URL]

// HTTPClient the default http client.
//
//nolint:exhaustivestruct,gochecknoglobals // ignore this
var HTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:               proxyFromConfig,
		MaxIdleConns:        DefaultMaxIdleConns,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		MaxConnsPerHost:     DefaultMaxConnsPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
	},
	Timeout: DefaultRequestTimeout,
	Jar:     jar,
}

// proxyFromConfig returns the configured proxy, or nil when none is set.
func proxyFromConfig(_ *http.Request) (*url.URL, error) {
	proxy := configuredProxy.Load()
	if proxy == nil {
		return nil, nil
	}

	return proxy, nil
}

// SetProxy configures the shared HTTPClient to use the given HTTP(S) proxy URL.
// An empty value clears the proxy. The URL must include a scheme such as
// "http://127.0.0.1:7890".
func SetProxy(proxyURL string) error {
	parsed, err := ParseProxyURL(proxyURL)
	if err != nil {
		return err
	}

	configuredProxy.Store(parsed)

	return nil
}

// ParseProxyURL validates an HTTP(S) proxy URL. An empty value returns nil.
func ParseProxyURL(proxyURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(proxyURL)
	if trimmed == "" {
		return nil, nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid http proxy %q: %w", trimmed, err)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid http proxy %q: scheme and host are required", trimmed)
	}

	return parsed, nil
}

// CurrentProxy returns the configured proxy URL string, or empty when none is set.
func CurrentProxy() string {
	proxy := configuredProxy.Load()
	if proxy == nil {
		return ""
	}

	return proxy.String()
}

//nolint:gochecknoglobals //ignore this.
var defaultHTTPHeaders = map[string]string{
	"accept":          "*/*",
	"accept-language": "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7",
	"user-agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/113.0.0.0 Safari/537.3",
	"mode":            "cors",
}

//nolint:gochecknoglobals //ignore this.
var wechatHTTPHeaders = map[string]string{
	"user-agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) MicroMessenger/6.8.0(0x16080000) MacWechat/3.4.1(0x13040110) Safari/605.1.15 NetType/WIFI",
}

func GetLinkData(link string) ([]byte, error) {
	return getWithHeaders(link, defaultHTTPHeaders)
}

// GetWechatLinkData 添加固定头部并不能获取微信公众号信息.
func GetWechatLinkData(link string) ([]byte, error) {
	return getWithHeaders(link, wechatHTTPHeaders)
}

func getWithHeaders(link string, headers map[string]string) ([]byte, error) {
	httpReq, err := http.NewRequest(http.MethodGet, link, bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var contentReader io.Reader = resp.Body

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "charset") {
		contentReader, err = charset.NewReader(contentReader, contentType)
		if err != nil {
			return nil, err
		}
	}

	return io.ReadAll(contentReader)
}
