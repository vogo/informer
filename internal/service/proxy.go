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

package service

import (
	"fmt"
	"strings"

	"github.com/vogo/logger"

	"github.com/vogo/informer/internal/configstore"
	"github.com/vogo/informer/internal/httpx"
)

// httpProxyKey is the top level key holding the HTTP(S) proxy inside informer.json.
// It is a plain network setting rather than a credential, so it lives next to
// the other shareable settings and never enters informer.secret.json.
const httpProxyKey = "http_proxy"

// SaveHTTPProxy stores the HTTP(S) proxy in informer.json and applies it to the
// shared HTTP client. An empty value clears it.
func (s *Service) SaveHTTPProxy(proxy string) error {
	trimmed := strings.TrimSpace(proxy)

	_, err := httpx.ParseProxyURL(trimmed)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidArgument, err)
	}

	err = configstore.WithLock(s.ConfigFilePath(), configstore.DefaultLockTimeout, func() error {
		doc, loadErr := configstore.Load(s.ConfigFilePath())
		if loadErr != nil {
			return loadErr
		}

		if trimmed == "" {
			doc.Delete(httpProxyKey)
		} else {
			setErr := doc.Set(httpProxyKey, trimmed)
			if setErr != nil {
				return setErr
			}
		}

		data, bytesErr := doc.Bytes()
		if bytesErr != nil {
			return bytesErr
		}

		return configstore.WriteAtomic(s.ConfigFilePath(), data, configstore.PermConfig)
	})
	if err != nil {
		return err
	}

	return httpx.SetProxy(trimmed)
}

// ApplyHTTPProxy reads the configured proxy and installs it on the shared HTTP
// client. A missing or empty value clears the proxy. Failures are logged rather
// than returned so a broken proxy line cannot block an inform run that may still
// succeed without one.
func (s *Service) ApplyHTTPProxy() {
	proxy, err := s.readHTTPProxy()
	if err != nil {
		logger.Warnf("read http_proxy failed: %v", err)

		return
	}

	err = httpx.SetProxy(proxy)
	if err != nil {
		logger.Warnf("apply http_proxy failed: %v", err)
	}
}

// readHTTPProxy returns the stored HTTP(S) proxy from informer.json.
func (s *Service) readHTTPProxy() (string, error) {
	doc, err := configstore.Load(s.ConfigFilePath())
	if err != nil {
		return "", err
	}

	var proxy string

	_, err = doc.Unmarshal(httpProxyKey, &proxy)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(proxy), nil
}
