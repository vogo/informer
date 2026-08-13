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

package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/httpx"
	"github.com/vogo/informer/internal/inform"
	"github.com/vogo/informer/internal/service"
)

func TestSaveHTTPProxyWritesAndApplies(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, httpx.SetProxy(""))
	})

	svc := newService(t)
	const proxy = "http://127.0.0.1:7890"

	require.NoError(t, svc.SaveHTTPProxy(proxy))

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.Equal(t, proxy, view.HTTPProxy)
	assert.Equal(t, proxy, readConfigFile(t, svc)["http_proxy"])
	assert.Equal(t, proxy, httpx.CurrentProxy())

	resolved := svc.EffectiveAgentConfig(&inform.Config{})
	assert.Equal(t, proxy, resolved.HTTPProxy)
}

func TestSaveHTTPProxyClearsTheStoredValue(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, httpx.SetProxy(""))
	})

	svc := newService(t)
	require.NoError(t, svc.SaveHTTPProxy("http://127.0.0.1:7890"))
	require.NoError(t, svc.SaveHTTPProxy(""))

	view, err := svc.ReadFileConfigView()
	require.NoError(t, err)
	assert.Empty(t, view.HTTPProxy)
	assert.NotContains(t, readConfigFile(t, svc), "http_proxy")
	assert.Empty(t, httpx.CurrentProxy())
	assert.Empty(t, svc.EffectiveAgentConfig(&inform.Config{}).HTTPProxy)
}

func TestSaveHTTPProxyRejectsAnInvalidURL(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, httpx.SetProxy(""))
	})

	svc := newService(t)

	require.ErrorIs(t, svc.SaveHTTPProxy("not-a-url"), service.ErrInvalidArgument)
	assert.NotContains(t, readConfigFile(t, svc), "http_proxy")
	assert.Empty(t, httpx.CurrentProxy())
}

func TestApplyHTTPProxyReadsFromInformerJSON(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, httpx.SetProxy(""))
	})

	svc := newService(t)
	writeConfigFile(t, svc.HomeDir(), &inform.Config{
		HTTPProxy: "http://10.0.0.1:8080",
	})

	svc.ApplyHTTPProxy()
	assert.Equal(t, "http://10.0.0.1:8080", httpx.CurrentProxy())
}
