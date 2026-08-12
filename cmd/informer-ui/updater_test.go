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

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestInformerAssetMatcher(t *testing.T) {
	t.Parallel()

	assets := []github.ReleaseAsset{
		{Name: "informer-ui-v9.9.9-darwin-universal.dmg"},
		{Name: "informer-ui-v9.9.9-darwin-universal.app.zip"},
		{Name: "informer-ui-v9.9.9-windows-amd64-setup.exe"},
		{Name: "informer-ui-v9.9.9-windows-amd64.zip"},
		{Name: "informer-ui-v9.9.9-linux-amd64.tar.gz"},
		{Name: "informer-ui-v9.9.9-linux-amd64.deb"},
		{Name: "SHA256SUMS"},
	}

	tests := []struct {
		name string
		req  updater.CheckRequest
		want string
	}{
		{
			name: "darwin universal zip",
			req:  updater.CheckRequest{Platform: "darwin", Arch: "arm64"},
			want: "informer-ui-v9.9.9-darwin-universal.app.zip",
		},
		{
			name: "windows zip not setup",
			req:  updater.CheckRequest{Platform: "windows", Arch: "amd64"},
			want: "informer-ui-v9.9.9-windows-amd64.zip",
		},
		{
			name: "linux tar.gz not deb",
			req:  updater.CheckRequest{Platform: "linux", Arch: "amd64"},
			want: "informer-ui-v9.9.9-linux-amd64.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idx := informerAssetMatcher(tt.req, assets)
			require.GreaterOrEqual(t, idx, 0)
			require.Equal(t, tt.want, assets[idx].Name)
		})
	}
}
