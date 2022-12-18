package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
	"unsafe"
	"wxChatGPT/config"
	"wxChatGPT/convert"
	"wxChatGPT/httpcli"
	"wxChatGPT/util"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

var baseHeader = map[string]string{
	"User-Agent":                "",
	"Accept":                    "text/event-stream",
	"Content-Type":              "application/json",
	"Connection":                "close",
	"Authorization":             "",
	"Referer":                   "https://chat.openai.com/chat",
	"Origin":                    "https://chat.openai.com",
	"x-openai-assistant-app-id": "",
}

type UserInfo struct {
	parentID       string
	conversationId interface{}
	TTL            time.Time
}

func NewUserInfo() *UserInfo {
	return &UserInfo{
		parentID:       uuid.New().String(),
		conversationId: nil,
	}
}

func (user *UserInfo) SendMsg(ctx context.Context, authorization string, config *config.Config, msg string) string {
	req, err := http.NewRequestWithContext(ctx, "POST", "https://chat.openai.com/backend-api/conversation", convert.CreateChatReqBody(msg, user.parentID, user.conversationId))
	if err != nil {
		panic(err)
	}
	baseHeader["Authorization"] = "Bearer " + authorization
	baseHeader["User-Agent"] = config.UserAgent
	for k, v := range baseHeader {
		req.Header.Set(k, v)
	}
	req.AddCookie(&http.Cookie{
		Name:  "cf_clearance",
		Value: config.CfClearance,
	})
	resp, err := httpcli.SkipTLSVerify.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	bodyBytes, err := util.ReadWithCtx(ctx, resp.Body)
	defer util.PutBytes(bodyBytes)
	if err != nil {
		panic(err)
	}
	line := bytes.Split(bodyBytes, []byte("\n\n"))
	if len(line) < 2 {
		log.Debugf("body %s", string(bodyBytes))
		panic(fmt.Errorf("%s", *(*string)(unsafe.Pointer(&bodyBytes))))
	}
	endBlock := line[len(line)-3][6:]
	res := convert.ToChatRes(endBlock)
	user.conversationId = res.ConversationId
	user.parentID = res.Message.Id
	return res.Message.Content.Parts[0]
}
