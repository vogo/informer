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

package informer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

func getDailySoup() string {
	resp, err := http.Get("http://open.iciba.com/dsapi/")
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return ""
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return ""
	}
	if resp.StatusCode != 200 {
		fmt.Printf("err: %d, %s\n", resp.StatusCode, b)
		return ""
	}

	data := struct {
		Content string
	}{}

	if err = json.Unmarshal(b, &data); err != nil {
		fmt.Printf("err: %v\n", err)
		return ""
	}

	return data.Content
}
