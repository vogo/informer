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

package feed_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wongoo/informer/internal/feed"
)

func TestFormatURL(t *testing.T) {
	t.Parallel()

	checks := [][2]string{
		{"https://www.baidu.com/abc?utm_source=aaa", "https://www.baidu.com/abc"},
		{"https://www.baidu.com/abc?utm_source=aaa&p=1", "https://www.baidu.com/abc?p=1"},
	}

	for _, item := range checks {
		result, ok := feed.FormatURL(item[0])
		if !ok {
			t.Error("format link failed.", item[0])

			continue
		}

		assert.Equal(t, item[1], result)
	}
}
