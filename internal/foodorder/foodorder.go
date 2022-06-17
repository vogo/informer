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
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/vogo/logger"
	"github.com/vogo/vogo/vrand"
	"github.com/wongoo/informer/internal"
)

const (
	previousChosenFileName = "previous_chosen.json"
)

// Menu 菜单.
type Menu struct {
	Type     string   `json:"type"`
	ChoseNum int      `json:"chose_num"`
	List     []string `json:"list"`
}

// FoodConfig 点餐配置.
type FoodConfig struct {
	Partners    []string      `json:"partners"`
	Restaurants []*Restaurant `json:"restaurants"`
}

// Restaurant 餐厅.
type Restaurant struct {
	Name  string  `json:"name"`
	Tel   string  `json:"tel"`
	Menus []*Menu `json:"food_list"`
}

// Order 下单.
type Order struct {
	RestaurantName string              `json:"name"`
	User           string              `json:"user"`
	Chose          map[string][]string `json:"chose"`
}

func AddFoodAutoChose(buf *bytes.Buffer, foodConfig *FoodConfig, exeDir string) {
	var previousFoodOrders []*Order

	previousData, err := os.ReadFile(filepath.Join(exeDir, previousChosenFileName))
	if err != nil {
		logger.Infof("read previous chosen error: %v", err)
	}

	_ = json.Unmarshal(previousData, &previousFoodOrders)

	var orderUser string
	if len(foodConfig.Partners) > 0 {
		orderUser = foodConfig.Partners[0]
	}

	if len(previousFoodOrders) > 0 {
		previousLatestOrder := previousFoodOrders[len(previousFoodOrders)-1]
		if previousLatestOrder.User != "" {
			for i, u := range foodConfig.Partners {
				if u == previousLatestOrder.User && i < len(foodConfig.Partners)-1 {
					orderUser = foodConfig.Partners[i+1]
				}
			}
		}
	}

	logger.Infof("order user: %s", orderUser)

	autoChoseFood(buf, exeDir, foodConfig, previousFoodOrders, &orderUser)
}

func autoChoseFood(buf *bytes.Buffer, exeDir string, foodConfig *FoodConfig, previousFoodOrders []*Order, orderUser *string) {
	rand.Seed(time.Now().Unix())

	if len(previousFoodOrders) == len(foodConfig.Restaurants) {
		previousFoodOrders = nil
	}

	restaurants := filterPreviousChosenRestaurants(previousFoodOrders, foodConfig.Restaurants)

	//nolint:gosec // ignore this
	restaurantIndex := rand.Intn(len(restaurants))
	restaurant := restaurants[restaurantIndex]

	buf.WriteString("中午为你推荐餐厅《" + restaurant.Name + "》")

	if restaurant.Tel != "" {
		buf.WriteString("(点餐电话" + restaurant.Tel + ")")
	}

	rand.Seed(time.Now().Unix())

	foodOrder := &Order{
		RestaurantName: restaurant.Name,
		User:           *orderUser,
		Chose:          make(map[string][]string),
	}

	buf.WriteString("\n\n")

	if len(restaurant.Menus) == 0 {
		*orderUser = ""
	} else {
		for _, foodMenu := range restaurant.Menus {
			buf.WriteString(foodMenu.Type)
			buf.WriteByte(':')
			for i := 0; i < foodMenu.ChoseNum; i++ {
				index := vrand.Intn(len(foodMenu.List))
				if i > 0 {
					buf.WriteByte(',')
				}
				buf.WriteString(foodMenu.List[index])

				foodOrder.Chose[foodMenu.Type] = append(foodOrder.Chose[foodMenu.Type], foodMenu.List[index])
				foodMenu.List = append(foodMenu.List[:index], foodMenu.List[index+1:]...)
			}
			buf.WriteByte('\n')
		}

		if restaurant.Tel != "" {
			buf.WriteString("\n需要一起点餐的同学+1, 被@的同学负责点餐~\n")
		}
	}

	previousFoodOrders = append(previousFoodOrders, foodOrder)
	if b, err := json.Marshal(previousFoodOrders); err == nil {
		_ = os.WriteFile(filepath.Join(exeDir, previousChosenFileName), b, internal.DefaultDataFilePermission)
	}
}

func filterPreviousChosenRestaurants(previousFoodOrders []*Order, restaurants []*Restaurant) []*Restaurant {
	if len(previousFoodOrders) == 0 {
		return restaurants
	}

	var results []*Restaurant
LOOP1:
	for _, r := range restaurants {
		for _, t := range previousFoodOrders {
			if t.RestaurantName == r.Name {
				continue LOOP1
			}
		}
		results = append(results, r)
	}

	if len(results) == 0 {
		return restaurants
	}

	return results
}

//nolint:deadcode,unused // ignore this
func filterPreviousChosenMenus(previousFoodOrders []*Order, restaurant *Restaurant) {
	var previousFoodOrder *Order

	for _, o := range previousFoodOrders {
		if o.RestaurantName == restaurant.Name {
			previousFoodOrder = o

			break
		}
	}

	if previousFoodOrder == nil || len(previousFoodOrder.Chose) == 0 {
		return
	}

	logger.Infof("previous chosen: %v", previousFoodOrder.Chose)

	for key := range previousFoodOrder.Chose {
		for _, foodMenu := range restaurant.Menus {
			if foodMenu.Type == key {
				filterItems(foodMenu, previousFoodOrder.Chose[key])

				break
			}
		}
	}
}

// nolint:unused // ignore this
func filterItems(menu *Menu, filters []string) {
	for _, filter := range filters {
		for index, name := range menu.List {
			if name == filter {
				menu.List = append(menu.List[:index], menu.List[index+1:]...)

				break
			}
		}
	}

	logger.Infof("after filter for %s: %s", menu.Type, menu.List)
}
