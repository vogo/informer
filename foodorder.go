package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	configFileName         = "foodorder.json"
	previousChosenFileName = "previous_chosen.json"
)

type FoodMenu struct {
	Type     string   `json:"type"`
	ChoseNum int      `json:"chose_num"`
	List     []string `json:"list"`
}

type FoodList []*FoodMenu

func main() {
	if len(os.Args) < 2 {
		log.Fatal("require ding url")
	}

	buf := bytes.NewBuffer(nil)

	now := time.Now()
	weekday := time.Now().Weekday()

	dateString := fmt.Sprintf("今天是 %s %s\n\n", now.Format("2006-01-02"), weekday.String())
	buf.WriteString(dateString)

	if dailySoup := GetDailySoup(); dailySoup != "" {
		buf.WriteString(dailySoup)
		buf.WriteByte('\n')
		buf.WriteByte('\n')
	}

	if weekday == time.Sunday {
		buf.WriteString("周末愉快!")
	} else {
		autoChoseFood(buf)
	}

	content := string(buf.Bytes())
	fmt.Print(content)
	ding(os.Args[1], content)
}

func autoChoseFood(buf *bytes.Buffer) {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	dataPath := filepath.Join(exeDir, configFileName)
	data, err := ioutil.ReadFile(dataPath)
	if err != nil {
		log.Fatal(err)
	}

	var foodList FoodList
	if err := json.Unmarshal(data, &foodList); err != nil {
		log.Fatal(err)
	}

	filterPreviousChosen(exeDir, &foodList)

	buf.WriteString("上班辛苦了! 中午为你推荐以下菜单: \n\n")

	rand.Seed(time.Now().Unix())

	chose := make(map[string][]string)
	for _, foodMenu := range foodList {
		buf.WriteString(foodMenu.Type)
		buf.WriteByte(':')
		for i := 0; i < foodMenu.ChoseNum; i++ {
			index := rand.Intn(len(foodMenu.List))
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(foodMenu.List[index])

			chose[foodMenu.Type] = append(chose[foodMenu.Type], foodMenu.List[index])
			foodMenu.List = append(foodMenu.List[:index], foodMenu.List[index+1:]...)
		}
		buf.WriteByte('\n')
	}

	if b, err := json.Marshal(chose); err == nil {
		_ = ioutil.WriteFile(filepath.Join(exeDir, previousChosenFileName), b, 0660)
	}
}

func filterPreviousChosen(exeDir string, foodList *FoodList) {
	previousChosen := make(map[string][]string)
	previousData, err := ioutil.ReadFile(filepath.Join(exeDir, previousChosenFileName))
	if err != nil {
		return
	}

	_ = json.Unmarshal(previousData, &previousChosen)

	if len(previousChosen) == 0 {
		return
	}

	log.Printf("previous chosen: %v", previousChosen)

	for key := range previousChosen {
		for _, foodMenu := range *foodList {
			if foodMenu.Type == key {
				filterItems(foodMenu, previousChosen[key])
				break
			}
		}
	}
}

func filterItems(foodMenu *FoodMenu, filters []string) {
	for _, filter := range filters {
		for index, name := range foodMenu.List {
			if name == filter {
				foodMenu.List = append(foodMenu.List[:index], foodMenu.List[index+1:]...)
				break
			}
		}
	}

	log.Printf("after filter for %s: %s", foodMenu.Type, foodMenu.List)
}

type DingText struct {
	Content string `json:"content"`
}
type DingMsg struct {
	MsgType string   `json:"msgtype"`
	Text    DingText `json:"text"`
}

func ding(url, content string) {
	msg := &DingMsg{
		MsgType: "text",
		Text: DingText{
			Content: content,
		},
	}
	data, _ := json.Marshal(msg)
	log.Printf("ding url: %s", url)
	log.Printf("ding data: %s", data)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("ding response: %v", resp)
}

func GetDailySoup() string {
	resp, err := http.Get("http://open.iciba.com/dsapi/")
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return ""
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return ""
	}
	if resp.StatusCode != 200 {
		fmt.Printf("err: %d, %s\n", resp.StatusCode, b)
		return ""
	}

	data := struct {
		Content string
	}{}

	if err = json.Unmarshal(b, &data); err != nil {
		fmt.Printf("err: %v\n", err)
		return ""
	}

	return data.Content
}
