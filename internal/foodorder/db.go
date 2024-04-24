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

package foodorder

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var foodorderDB *gorm.DB

func InitFoodorderDB(dataDir string) {
	var err error
	foodorderDB, err = gorm.Open(sqlite.Open(dataDir+"/foodorder.db"), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	if err = foodorderDB.AutoMigrate(&FoodConfig{}); err != nil {
		panic(err)
	}

	if err = foodorderDB.AutoMigrate(&Menu{}); err != nil {
		panic(err)
	}

	if err = foodorderDB.AutoMigrate(&MenuItem{}); err != nil {
		panic(err)
	}

	if err = foodorderDB.AutoMigrate(&Restaurant{}); err != nil {
		panic(err)
	}

	if err = foodorderDB.AutoMigrate(&Order{}); err != nil {
		panic(err)
	}

	if err = foodorderDB.AutoMigrate(&OrderItem{}); err != nil {
		panic(err)
	}

	if err = foodorderDB.AutoMigrate(&User{}); err != nil {
		panic(err)
	}
}
