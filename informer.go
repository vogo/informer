package informer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	data, err := ioutil.ReadFile(dataPath)
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

	content := string(buf.Bytes())
	fmt.Print(content)

	if len(os.Args) > 1 {
		// ding(os.Args[1], content, orderUser, weekday)
		lark(os.Args[1], content)
	}
}
