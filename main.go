package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"wxChatGPT/chatGPT"
	"wxChatGPT/config"
	"wxChatGPT/convert"
	"wxChatGPT/translate"
	"wxChatGPT/util"
	"wxChatGPT/util/middleware"
	"wxChatGPT/util/signature"

	"github.com/go-chi/chi/v5"
	m "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

const wxToken = "" // 这里填微信开发平台里设置的 Token

var reqGroup singleflight.Group

func init() {
	log.SetLevel(config.GetLogLevel())
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.TextFormatter{
		DisableColors:   runtime.GOOS == "windows",
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	config.AddConfigChangeCallback(func() {
		log.SetLevel(config.GetLogLevel())
	})
}

func main() {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recover)

	r.Post("/hcfyCustom", hcfyCustom)

	// ChatGPT 可用性检查
	r.Get("/healthCheck", healthCheck)
	// 微信接入校验
	r.Get("/weChatGPT", wechatCheck)
	// 微信消息处理
	r.Post("/weChatGPT", wechatMsgReceive)

	l, err := net.Listen("tcp", ":7458")
	if err != nil {
		log.Fatalln(err)
	}
	log.Infof("Server listening at %s", l.Addr())
	if err = http.Serve(l, r); err != nil {
		log.Fatalln(err)
	}
}

func hcfyCustom(w http.ResponseWriter, r *http.Request) {
	req := &translate.TranslateReq{}
	if err := render.Bind(r, req); err != nil {
		log.Debugf("bad request: %s", err)
		render.Status(r, http.StatusBadRequest)
		render.PlainText(w, r, err.Error())
		return
	}
	ch := make(chan *translate.TranslateResult, 1)
	translate.Translate(req, ch)
	select {
	case <-r.Context().Done():
		log.Errorf("context break, reason: %s, req: %+v", r.Context().Err(), req)
		render.Status(r, http.StatusInternalServerError)
		render.PlainText(w, r, r.Context().Err().Error())
		return
	case result := <-ch:
		if result.Err != nil {
			render.Status(r, http.StatusInternalServerError)
			render.PlainText(w, r, result.Err.Error())
			return
		} else {
			render.JSON(w, r, result.Resp)
			return
		}
	}
}

// ChatGPT 可用性检查
func healthCheck(w http.ResponseWriter, r *http.Request) {
	defer chatGPT.DefaultGPT().DeleteUser("healthCheck")
	msg, err, _ := reqGroup.Do("healthCheck", func() (interface{}, error) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := <-chatGPT.DefaultGPT().SendMsgChan("宇宙的终极答案是什么?", "healthCheck", ctx)
		return result.Val, result.Err
	})
	if err != nil {
		panic(err)
	}
	log.Infof("测试返回：%s", msg)
	render.PlainText(w, r, "ok")
}

// 微信接入校验
func wechatCheck(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	sign := query.Get("signature")
	timestamp := query.Get("timestamp")
	nonce := query.Get("nonce")
	echostr := query.Get("echostr")

	// 校验
	if signature.CheckSignature(sign, timestamp, nonce, wxToken) {
		render.PlainText(w, r, echostr)
		return
	}

	log.Warnln("微信接入校验失败")
}

// 微信消息处理
func wechatMsgReceive(w http.ResponseWriter, r *http.Request) {
	// 解析消息
	body, _ := io.ReadAll(r.Body)
	xmlMsg := convert.ToTextMsg(body)

	log.Infof("[消息接收] Type: %s, From: %s, MsgId: %d, Content: %s", xmlMsg.MsgType, xmlMsg.FromUserName, xmlMsg.MsgId, xmlMsg.Content)

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// 回复消息
	replyMsg := ""

	// 关注公众号事件
	if xmlMsg.MsgType == "event" {
		if xmlMsg.Event == "unsubscribe" {
			chatGPT.DefaultGPT().DeleteUser(xmlMsg.FromUserName)
		}
		if xmlMsg.Event != "subscribe" {
			util.TodoEvent(w)
			return
		}
		replyMsg = ":) 感谢你发现了这里"
	} else if xmlMsg.MsgType == "text" {
		// 【收到不支持的消息类型，暂无法显示】
		if strings.Contains(xmlMsg.Content, "【收到不支持的消息类型，暂无法显示】") {
			util.TodoEvent(w)
			return
		}
		// 最多等待 15 s， 超时返回空值
		msg, err, _ := reqGroup.Do(strconv.FormatInt(xmlMsg.MsgId, 10), func() (interface{}, error) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			select {
			case result := <-chatGPT.DefaultGPT().SendMsgChan(xmlMsg.Content, xmlMsg.FromUserName, ctx):
				return result.Val, result.Err
			case <-time.After(14*time.Second + 500*time.Millisecond):
				// 超时返回错误
				return "", fmt.Errorf("请求超时, MsgId: %d", xmlMsg.MsgId)
			}
		})
		if err != nil {
			panic(err)
		}
		replyMsg = msg.(string)
	} else {
		util.TodoEvent(w)
		return
	}

	textRes := &convert.TextRes{
		ToUserName:   xmlMsg.FromUserName,
		FromUserName: xmlMsg.ToUserName,
		CreateTime:   time.Now().Unix(),
		MsgType:      "text",
		Content:      replyMsg,
	}
	_, err := w.Write(textRes.ToXml())
	if err != nil {
		log.Errorln(err)
		if config.GetIsDebug() {
			m.PrintPrettyStack(err)
		}
	}
}
