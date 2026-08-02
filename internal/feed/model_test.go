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
	"github.com/vogo/informer/internal/feed"
)

func TestResolveParseType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		source feed.Source
		want   string
	}{
		{
			name:   "explicit value wins over the legacy fields",
			source: feed.Source{ParseType: feed.ParseTypeFeed, IsJSON: true, Regex: "x"},
			want:   feed.ParseTypeFeed,
		},
		{
			name:   "explicit regex wins over IsJSON",
			source: feed.Source{ParseType: feed.ParseTypeRegex, IsJSON: true},
			want:   feed.ParseTypeRegex,
		},
		{
			name:   "empty value derives json from IsJSON",
			source: feed.Source{IsJSON: true, Regex: "x"},
			want:   feed.ParseTypeJSON,
		},
		{
			name:   "empty value derives regex from a non empty Regex",
			source: feed.Source{Regex: "x"},
			want:   feed.ParseTypeRegex,
		},
		{
			name:   "empty value falls back to feed",
			source: feed.Source{},
			want:   feed.ParseTypeFeed,
		},
		{
			name:   "an unknown historical value also falls back",
			source: feed.Source{ParseType: "rss-v9", Regex: "x"},
			want:   feed.ParseTypeRegex,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, testCase.source.ResolveParseType())
		})
	}
}

func TestIsLegalParseType(t *testing.T) {
	t.Parallel()

	for _, legal := range []string{feed.ParseTypeFeed, feed.ParseTypeRegex, feed.ParseTypeJSON} {
		assert.True(t, feed.IsLegalParseType(legal), legal)
	}

	for _, illegal := range []string{"", "FEED", "rss", "xml"} {
		assert.False(t, feed.IsLegalParseType(illegal), illegal)
	}
}
