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

package httpx_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vogo/informer/internal/httpx"
)

func TestSetProxyAcceptsAValidURL(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, httpx.SetProxy(""))
	})

	require.NoError(t, httpx.SetProxy("http://127.0.0.1:7890"))
	assert.Equal(t, "http://127.0.0.1:7890", httpx.CurrentProxy())

	proxy, err := httpx.HTTPClient.Transport.(*http.Transport).Proxy(nil)
	require.NoError(t, err)
	require.NotNil(t, proxy)
	assert.Equal(t, "http://127.0.0.1:7890", proxy.String())
}

func TestSetProxyClearsWhenEmpty(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, httpx.SetProxy(""))
	})

	require.NoError(t, httpx.SetProxy("http://127.0.0.1:7890"))
	require.NoError(t, httpx.SetProxy(""))
	assert.Empty(t, httpx.CurrentProxy())

	proxy, err := httpx.HTTPClient.Transport.(*http.Transport).Proxy(nil)
	require.NoError(t, err)
	assert.Nil(t, proxy)
}

func TestSetProxyRejectsAnInvalidURL(t *testing.T) {
	t.Cleanup(func() {
		require.NoError(t, httpx.SetProxy(""))
	})

	require.Error(t, httpx.SetProxy("not-a-url"))
	require.Error(t, httpx.SetProxy("://missing-host"))
	assert.Empty(t, httpx.CurrentProxy())
}
