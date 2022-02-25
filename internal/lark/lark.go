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

package lark

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

const Host = "feishu.cn"

type Content struct {
	Text string `json:"text"`
}

type Message struct {
	Type    string   `json:"msg_type"`
	Content *Content `json:"content"`
}

func Lark(url, content string) {
	msg := &Message{
		Type: "text",
		Content: &Content{
			Text: content,
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Println(err)
	}

	log.Printf("lark url: %s", url)
	log.Printf("lark data: %s", data)

	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	log.Printf("lark response: %v", resp)
}
