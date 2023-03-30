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

	"github.com/vogo/informer/internal/date"
	"github.com/vogo/informer/internal/ding"
	"github.com/vogo/informer/internal/feed"
	"github.com/vogo/informer/internal/foodorder"
	"github.com/vogo/informer/internal/lark"
	"github.com/vogo/informer/internal/soup"
	"github.com/vogo/logger"
)

const (
	configFileName = "informer.json"
)

type Config struct {
	Feed *feed.Config `json:"feed"`
}

func Inform(exeDir, urlAddr string) {
	buf := bytes.NewBuffer(nil)

	buf.WriteString(date.GetDateInfo())

	if dailySoup := soup.GetDailySoup(); dailySoup != "" {
		buf.WriteString(dailySoup)
		buf.WriteByte('\n')
		buf.WriteByte('\n')
	}

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

	foodorder.InitFoodorderDB(exeDir)
	foodConfigs := foodorder.GetAllFoodConfig()
	if len(foodConfigs) <= 0 {
		logger.Info("No food config found, Init food config from informer.json")
		// 初始化数据库
		foodorder.InitFoodOrderData(data)
		foodConfigs = foodorder.GetAllFoodConfig()
	}
	for _, foodConfig := range foodConfigs {
		if foodConfig != nil && weekday != time.Sunday && weekday != time.Saturday {
			foodorder.AddFoodAutoChose(buf, foodConfig, exeDir)
		}
	}

	if informerConfig.Feed != nil {
		feed.InitFeedDB(exeDir)
		feed.AddFeeds(buf, informerConfig.Feed)
	}

	content := buf.String()
	logger.Info(content)

	if urlAddr != "" {
		if strings.Contains(urlAddr, lark.Host) {
			lark.Lark(urlAddr, content)
		} else if strings.Contains(urlAddr, ding.Host) {
			ding.Ding(urlAddr, content, "", weekday)
		}
	}
}
