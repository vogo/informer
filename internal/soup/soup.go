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

package soup

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/vogo/logger"
)

func GetDailySoup() string {
	resp, err := http.Get("http://open.iciba.com/dsapi/")
	if err != nil {
		logger.Infof("err: %v", err)

		return ""
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Infof("err: %v", err)

		return ""
	}

	if resp.StatusCode != http.StatusOK {
		logger.Infof("err: %d, %s", resp.StatusCode, b)

		return ""
	}

	data := struct {
		Content string
	}{}

	if err = json.Unmarshal(b, &data); err != nil {
		logger.Infof("err: %v", err)

		return ""
	}

	return data.Content
}
