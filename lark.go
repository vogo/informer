package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type LarkContent struct {
	Text string `json:"text"`
}

type LarkMessage struct {
	Type    string       `json:"msg_type"`
	Content *LarkContent `json:"content"`
}

func lark(url, content, _ string, _ time.Weekday) {
	msg := &LarkMessage{
		Type: "text",
		Content: &LarkContent{
			Text: content,
		},
	}

	data, _ := json.Marshal(msg)
	log.Printf("lark url: %s", url)
	log.Printf("lark data: %s", data)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("lark response: %v", resp)
}
