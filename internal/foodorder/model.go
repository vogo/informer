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

// Menu 菜单.
type Menu struct {
	ID           string `json:"id" gorm:"primarykey"`
	Type         string `json:"type"`
	ChoseNum     int    `json:"chose_num"`
	RestaurantId int64  `json:"restaurant_id"`
}

// MenuItem 菜单项
type MenuItem struct {
	ID     int64   `json:"id" gorm:"primarykey;AUTO_INCREMENT"`
	Name   string  `json:"name"`
	Price  float64 `json:"price"`
	MenuId string  `json:"menu_id"`
}

// FoodConfig 点餐配置.
type FoodConfig struct {
	ID       int64    `json:"id" gorm:"primarykey;AUTO_INCREMENT"`
	Partners []string `json:"partners"`
}

// Restaurant 餐厅.
type Restaurant struct {
	ID   int64  `json:"id" gorm:"primarykey;AUTO_INCREMENT"`
	Name string `json:"name"`
	Tel  string `json:"tel"`
}

// Order 下单.
type Order struct {
	ID             string `json:"id" gorm:"primarykey;"`
	RestaurantId   int64  `json:"restaurant_id"`
	RestaurantName string `json:"restaurant_name"`
	UserId         int64  `json:"user_id"`
	Partners       string `json:"partners"`
}

// OrderItem 下单项
type OrderItem struct {
	ID         int64  `json:"id" gorm:"primarykey;AUTO_INCREMENT"`
	MenuItemId int64  `json:"menu_item_id"`
	OrderId    string `json:"order_id"`
}

// User 用户
type User struct {
	ID       int64  `json:"id" gorm:"primarykey;AUTO_INCREMENT"`
	Name     string `json:"name"`
	MobileNo string `json:"mobile_no"`
}
