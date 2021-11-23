package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type DingText struct {
	Content string `json:"content"`
}

type DingAt struct {
	AtMobiles []string `json:"atMobiles"`
}

type DingMsg struct {
	MsgType string   `json:"msgtype"`
	Text    DingText `json:"text"`
	At      DingAt   `json:"at"`
}

func ding(url, content, user string, weekday time.Weekday) {
	msg := &DingMsg{
		MsgType: "text",
		Text: DingText{
			Content: content,
		},
	}

	if user != "" && weekday != time.Sunday && weekday != time.Saturday {
		msg.At = DingAt{AtMobiles: []string{user}}
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
