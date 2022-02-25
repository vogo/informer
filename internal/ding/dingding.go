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

package ding

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

const Host = "dingtalk.com"

type MsgText struct {
	Content string `json:"content"`
}

type MsgAt struct {
	AtMobiles []string `json:"atMobiles"`
}

type MsgBody struct {
	MsgType string  `json:"msgtype"`
	Text    MsgText `json:"text"`
	At      MsgAt   `json:"at"`
}

func Ding(url, content, user string, weekday time.Weekday) {
	msg := &MsgBody{
		MsgType: "text",
		Text: MsgText{
			Content: content,
		},
	}

	if user != "" && weekday != time.Sunday && weekday != time.Saturday {
		msg.At = MsgAt{AtMobiles: []string{user}}
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Println(err)
	}

	log.Printf("ding url: %s", url)
	log.Printf("ding data: %s", data)

	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	log.Printf("ding response: %v", resp)
}
