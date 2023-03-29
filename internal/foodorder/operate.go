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

func getAllFoodConfig() []*FoodConfig {
	var foodConfigs []*FoodConfig

	foodorderDB.Model(&FoodConfig{}).Order("id").Find(&foodConfigs)

	return foodConfigs
}

// 获取最近一次点餐记录
func getPreviousLatestOrder(user *User) *Order {
	var order Order

	foodorderDB.Model(&Order{UserId: user.ID}).Order("id desc").Last(&order)

	return &order
}

// 获取用户
func getUser(mobileNo string) *User {
	var user User

	foodorderDB.Model(&User{MobileNo: mobileNo}).First(&user)

	return &user
}
