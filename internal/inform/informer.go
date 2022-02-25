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

package inform

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wongoo/informer/internal/date"
	"github.com/wongoo/informer/internal/ding"
	"github.com/wongoo/informer/internal/feed"
	"github.com/wongoo/informer/internal/foodorder"
	"github.com/wongoo/informer/internal/lark"
	"github.com/wongoo/informer/internal/soup"
)

const (
	configFileName = "informer.json"
)

type Config struct {
	Food *foodorder.FoodConfig `json:"food"`
	Feed *feed.Config          `json:"feed"`
}

func Inform() {
	buf := bytes.NewBuffer(nil)

	buf.WriteString(date.GetDateInfo())

	if dailySoup := soup.GetDailySoup(); dailySoup != "" {
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
		foodorder.AddFoodAutoChose(buf, informerConfig.Food, exeDir)
	}

	feed.AddFeeds(buf, informerConfig.Feed, exeDir)

	content := buf.String()
	log.Println(content)

	if len(os.Args) > 1 {
		url := os.Args[1]
		if strings.Contains(url, lark.Host) {
			lark.Lark(url, content)
		} else if strings.Contains(url, ding.Host) {
			ding.Ding(url, content, "", weekday)
		}
	}
}
