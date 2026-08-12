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

const (
	testAssetDarwinZip  = "informer-ui-v9.9.9-darwin-universal.app.zip"
	testAssetWindowsZip = "informer-ui-v9.9.9-windows-amd64.zip"
	testAssetLinuxTar   = "informer-ui-v9.9.9-linux-amd64.tar.gz"
)

func TestInformerAssetMatcher(t *testing.T) {
	t.Parallel()

	assets := []github.ReleaseAsset{
		{Name: "informer-ui-v9.9.9-darwin-universal.dmg"},
		{Name: testAssetDarwinZip},
		{Name: "informer-ui-v9.9.9-windows-amd64-setup.exe"},
		{Name: testAssetWindowsZip},
		{Name: testAssetLinuxTar},
		{Name: "informer-ui-v9.9.9-linux-amd64.deb"},
		{Name: checksumAsset},
	}

	tests := []struct {
		name string
		req  updater.CheckRequest
		want string
	}{
		{
			name: "darwin universal zip",
			req:  updater.CheckRequest{Platform: platformDarwin, Arch: archARM64},
			want: testAssetDarwinZip,
		},
		{
			name: "windows zip not setup",
			req:  updater.CheckRequest{Platform: platformWindows, Arch: archAMD64},
			want: testAssetWindowsZip,
		},
		{
			name: "linux tar.gz not deb",
			req:  updater.CheckRequest{Platform: platformLinux, Arch: archAMD64},
			want: testAssetLinuxTar,
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
