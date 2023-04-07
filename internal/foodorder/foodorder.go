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
	"strconv"
	"strings"
	"time"

	"github.com/vogo/logger"
	"github.com/vogo/vogo/vrand"
)

const (
	previousChosenFileName = "previous_chosen.json"
)

// 初始化入库
func InitFoodOrderData(data []byte) {
	// 把data通过JSON反序列化成map
	var informerConfig map[string]interface{}

	if err := json.Unmarshal(data, &informerConfig); err != nil {
		logger.Fatal(err)
	}
	// 取出food的配置，转成map[string]interface{}
	foodConfig := informerConfig["food"].(map[string]interface{})
	// 取出food的配置中的partners，转成[]string
	partners := getStringArr(foodConfig, "partners")
	addFoodConfig(partners)
	addUser(partners)
	// 取出food的配置中的restaurants，转成[]map[string]interface{}
	restaurants := getMap(foodConfig, "restaurants")
	// 遍历restaurants
	for _, restaurant := range restaurants {
		// 取出restaurant的name，转成string
		name := getString(restaurant, "name")
		// 取出restaurant的tel，转成string

		tel := getString(restaurant, "tel")
		restaurantId := addRestaurant(name, tel).ID
		// 取出restaurant的menu，转成[]map[string]interface{}
		menus := getMap(restaurant, "food_list")
		// 遍历menu
		for _, menu := range menus {
			// 取出menu的type，转成string
			menuType := getString(menu, "type")
			// 取出menu的choseNum，转成int
			choseNum := getInt(menu, "chose_num")
			// 取出menu的list，转成[]string
			foodList := getStringArr(menu, "list")
			menu := addMenu(menuType, choseNum, restaurantId)
			menuItems := []*MenuItem{}
			// 遍历foodList
			for _, food := range foodList {
				// 把food按照空格分割成[]string
				foodInfo := strings.Split(food, " ")
				// 取出foodInfo的第一个元素，转成string
				foodName := foodInfo[0]
				// 取出foodInfo的第二个元素，转成float64
				price, _ := strconv.ParseFloat(foodInfo[1], 64)
				// 把name, foodName, price, choseNum, menuType, partners插入数据库
				menuItems = append(menuItems, buildMenuItem(menu, foodName, price))
			}

			foodorderDB.Create(menuItems)
		}
	}
}

func getString(data map[string]interface{}, key string) string {
	// 判断data里有没有key
	if _, ok := data[key]; ok {
		return data[key].(string)
	}

	return ""
}

func getStringArr(data map[string]interface{}, key string) []string {
	// 判断data里有没有key
	if _, ok := data[key]; ok {
		arr := data[key].([]interface{})

		var result []string

		for _, v := range arr {
			result = append(result, v.(string))
		}

		return result
	}

	return nil
}

func getInt(data map[string]interface{}, key string) int {
	// 判断data里有没有key
	if _, ok := data[key]; ok {
		return int(data[key].(float64))
	}
	return 0
}

func getMap(data map[string]interface{}, key string) []map[string]interface{} {
	if _, ok := data[key]; ok {
		arr := data[key].([]interface{})
		var result []map[string]interface{}

		for _, v := range arr {
			result = append(result, v.(map[string]interface{}))
		}

		return result
	}
	return nil
}

// 增加点餐配置
func addFoodConfig(partners []string) {
	partnersJson, _ := json.Marshal(partners)
	foodConfig := &FoodConfig{
		Partners: string(partnersJson),
	}
	foodorderDB.Create(foodConfig)
}

// 增加餐馆
func addRestaurant(name, tel string) *Restaurant {
	restaurant := &Restaurant{
		Name: name,
		Tel:  tel,
	}
	foodorderDB.Create(restaurant)
	return getRestaurantByName(name)
}

// 增加用户
func addUser(partners []string) {
	users := []*User{}

	for _, partner := range partners {
		user := &User{
			MobileNo: partner,
			Name:     partner,
		}
		users = append(users, user)
	}

	foodorderDB.Create(users)
}

// 增加菜单
func addMenu(menuType string, choseNum int, restaurantId int64) *Menu {
	menu := &Menu{
		ID:           generateMenuId(),
		Type:         menuType,
		ChoseNum:     choseNum,
		RestaurantId: restaurantId,
	}
	foodorderDB.Create(menu)
	return menu
}

// 构建菜单项
func buildMenuItem(menu *Menu, foodName string, price float64) *MenuItem {
	menuItem := &MenuItem{
		MenuId: menu.ID,
		Name:   foodName,
		Price:  price,
	}
	return menuItem
}

func AddFoodAutoChose(buf *bytes.Buffer, foodConfig *FoodConfig, exeDir string) {
	var orderUserMobileNo string
	if len(foodConfig.Partners) <= 0 {
		logger.Error("partners can not be empty")
		buf.WriteString("不能没有点餐人")
		return
	}
	var partners []string

	json.Unmarshal([]byte(foodConfig.Partners), &partners)
	orderUserMobileNo = partners[0]
	var orderUser *User = getUser(orderUserMobileNo)

	previousOrders := getPreviousOrder(orderUser)
	if len(previousOrders) > 0 {
		previousLatestOrder := previousOrders[len(previousOrders)-1]
		if previousLatestOrder.UserId > 0 {
			for i, u := range partners {
				if u == previousLatestOrder.Partners && i < len(foodConfig.Partners)-1 {
					// 上次点餐的人，这次换一个
					orderUser = getUser(partners[i+1])
				}
			}
		}
	}

	logger.Infof("order user: %s", orderUser.Name)

	autoChoseFood(buf, exeDir, foodConfig, previousOrders, orderUser)
}

func autoChoseFood(buf *bytes.Buffer, exeDir string, foodConfig *FoodConfig, previousFoodOrders []*Order, orderUser *User) {
	r := rand.New(rand.NewSource(time.Now().Unix()))

	restaurants := getAllRestaurants()
	if len(previousFoodOrders) == len(restaurants) {
		previousFoodOrders = nil
	}

	restaurants = filterPreviousChosenRestaurants(previousFoodOrders, restaurants)

	restaurantIndex := r.Intn(len(restaurants))
	restaurant := restaurants[restaurantIndex]

	buf.WriteString("中午为你推荐餐厅《" + restaurant.Name + "》")

	if restaurant.Tel != "" {
		buf.WriteString("(点餐电话" + restaurant.Tel + ")")
	}

	foodOrder := &Order{
		ID:           generateOrderId(),
		RestaurantId: restaurant.ID,
		UserId:       orderUser.ID,
		Partners:     orderUser.MobileNo,
	}

	orderItems := []*OrderItem{}

	buf.WriteString("\n\n")

	menus := getRestaurantMenu(restaurant.ID)
	if len(menus) == 0 {
	} else {
		for _, foodMenu := range menus {
			buf.WriteString(foodMenu.Type)
			buf.WriteByte(':')
			menuItems := getRestaurantMenuItemList(foodMenu.ID)
			randomChoseMenuItems := randomChoseMenuItems(menuItems, foodMenu.ChoseNum)
			for i, item := range randomChoseMenuItems {
				if i > 0 {
					buf.WriteByte(',')
				}
				orderItems = append(orderItems, &OrderItem{
					MenuItemId: item.ID,
					OrderId:    foodOrder.ID,
				})
				buf.WriteString(item.Name)
			}
			buf.WriteByte('\n')
		}

		if restaurant.Tel != "" {
			buf.WriteString("\n需要一起点餐的同学+1, 被@的同学负责点餐~\n")
		}
	}

	// 订单入库
	saveOrder(foodOrder)

	// 订单项入库
	saveOrderItemList(orderItems)
}

// 随机从N个菜单项中选取M个菜单项
func randomChoseMenuItems(menuItems []*MenuItem, choseNum int) []*MenuItem {
	if len(menuItems) <= choseNum {
		return menuItems
	}

	var results []*MenuItem

	for i := 0; i < choseNum; i++ {
		index := vrand.Intn(len(menuItems))
		results = append(results, menuItems[index])
		menuItems = append(menuItems[:index], menuItems[index+1:]...)
	}

	return results
}

func filterPreviousChosenRestaurants(previousFoodOrders []*Order, restaurants []*Restaurant) []*Restaurant {
	if len(previousFoodOrders) == 0 {
		return restaurants
	}

	var results []*Restaurant
LOOP1:
	for _, rest := range restaurants {
		for _, t := range previousFoodOrders {
			if t.RestaurantId == rest.ID {
				continue LOOP1
			}
		}
		results = append(results, rest)
	}

	if len(results) == 0 {
		return restaurants
	}

	return results
}

// 生成订单ID，规则是yyyyMMddHHmmss加上1000000以内的随机数
func generateOrderId() string {
	return time.Now().Format("20060102150405") + strconv.Itoa(vrand.Intn(1000000))
}

func generateMenuId() string {
	return time.Now().Format("20060102150405") + strconv.Itoa(vrand.Intn(1000000))
}
