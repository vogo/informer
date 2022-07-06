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

package util_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wongoo/informer/internal/util"
)

func TestGetMatchRender(t *testing.T) {
	t.Parallel()

	data := `id:123,title:abc; id:456,title:def; id:789,title:ghi`

	re := regexp.MustCompile("id:([^,]+),title:([^;]+)")

	match := re.FindAllSubmatch([]byte(data), -1)

	render := util.RegexMatchRender("https://www.example.com/$1?title=$2")

	assert.Equal(t, 3, len(match))
	assert.Equal(t, []byte("https://www.example.com/123?title=abc"), render(match[0]))
	assert.Equal(t, []byte("https://www.example.com/456?title=def"), render(match[1]))
	assert.Equal(t, []byte("https://www.example.com/789?title=ghi"), render(match[2]))
}
