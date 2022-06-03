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

package feed

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/vogo/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var feedDataDB *gorm.DB

func InitFeedDB(dataDir string) {
	var err error
	feedDataDB, err = gorm.Open(sqlite.Open(dataDir+"/feed.db"), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	if err = feedDataDB.AutoMigrate(&Article{}); err != nil {
		panic(err)
	}
}

const feedDataJSONFile = "feed_data.json"

func saveJsonDataToFeedDB(confDir string) {
	feedDateFilePath := filepath.Join(confDir, feedDataJSONFile)

	dataFile, err := os.ReadFile(feedDateFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Infof("read feed data error: %v", err)
		}

		return
	}

	feedData := make(map[string]*Article)

	_ = json.Unmarshal(dataFile, &feedData)

	for k, v := range feedData {
		v.URL = k

		if v.Score == 0 {
			v.Score = v.Weight
		}

		feedDataDB.Save(v)
	}

	_ = os.Remove(feedDateFilePath)
}
