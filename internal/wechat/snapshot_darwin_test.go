//go:build darwin && cgo

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

//nolint:testpackage //Exercise the private native bridge without changing the real UI.
package wechat

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// This opt-in regression exercises real Foundation JSON boxing and Go decoding.
// It only reads the current window; never navigate, edit drafts or send messages.
func TestNativeSnapshotUsesJSONBooleans(t *testing.T) {
	t.Parallel()
	if os.Getenv("INFORMER_TEST_WECHAT_SNAPSHOT") != "1" {
		t.Skip("set INFORMER_TEST_WECHAT_SNAPSHOT=1 with WeChat open and accessibility permission enabled")
	}
	result, err := platformDriver().call(request{Op: "snapshot"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Nodes)
	t.Logf("decoded %d native UI nodes with strict boolean fields", len(result.Nodes))
}

//nolint:paralleltest //Opt-in navigation operates the one real WeChat window.
func TestNativeLocateConfiguredGroup(t *testing.T) {
	group := os.Getenv("INFORMER_TEST_WECHAT_LOCATE_GROUP")
	if group == "" {
		t.Skip("set INFORMER_TEST_WECHAT_LOCATE_GROUP to an explicitly authorized group; this never sends")
	}
	result, err := Locate(group)
	require.NoError(t, err)
	require.Equal(t, "located", result.Status)
}
