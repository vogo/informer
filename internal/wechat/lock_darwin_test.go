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

//nolint:testpackage //Exercise the private driver and durable send state without operating the real UI.
package wechat

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLockDoesNotTakeOverActiveSenderAndReleases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, platformLock(dir, func(_ *os.File) error {
		err := platformLock(dir, func(_ *os.File) error {
			t.Fatal("overlapping UI operation")

			return nil
		})
		require.Error(t, err)

		return nil
	}))
	require.NoError(t, platformLock(dir, func(_ *os.File) error { return nil }))
}
