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
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	configFileName = "informer.json"
)

type Config struct {
	Food *FoodConfig `json:"food"`
	Feed *FeedConfig `json:"feed"`
}

func Inform() {
	buf := bytes.NewBuffer(nil)

	buf.WriteString(getDateInfo())

	if dailySoup := getDailySoup(); dailySoup != "" {
		buf.WriteString(dailySoup)
		buf.WriteByte('\n')
		buf.WriteByte('\n')
	}

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	dataPath := filepath.Join(exeDir, configFileName)

	data, err := os.ReadFile(dataPath)
	if err != nil {
		log.Fatal(err)
	}

	var informerConfig Config
	if err := json.Unmarshal(data, &informerConfig); err != nil {
		log.Fatal(err)
	}

	weekday := time.Now().Weekday()
	if weekday != time.Sunday && weekday != time.Saturday {
		addFoodAutoChose(buf, informerConfig, exeDir)
	}

	addFeeds(buf, informerConfig.Feed, exeDir)

	content := buf.String()
	log.Println(content)

	if len(os.Args) > 1 {
		lark(os.Args[1], content)
	}
}
