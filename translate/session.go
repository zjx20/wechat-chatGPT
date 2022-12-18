package translate

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"wxChatGPT/chatGPT"

	log "github.com/sirupsen/logrus"
)

const (
	thresholdSize = 1024
)

var (
	paraPattern = regexp.MustCompile(`(?ms)\s*<(.*?)->(.*?)>\s*(.*?)\z`)

	singleDestTemplate = template.Must(template.New("single_dest").Parse(`
下面有一段文本，以 ----begin---- 开始，以 ----end---- 结束。请把它翻译成{{index .Dest 0}}。输出的第一行首先写从哪个语种翻译到哪个语种，格式为 <{source} -> {destination}> ，语种用中文表达；紧接着写每段的翻译，同样用换行符分割。

----begin----
{{.Body}}
----end----
	`))

	multiDestTemplate = template.Must(template.New("multi_dest").Parse(`
下面有一段文本，以 ----begin---- 开始，以 ----end---- 结束。请把它翻译成{{index .Dest 0}}。如果它已经是{{index .Dest 0}}，则把它翻译成{{index .Dest 1}}。输出的第一行首先写从哪个语种翻译到哪个语种，格式为 <{source} -> {destination}> ，语种用中文表达；紧接着写每段的翻译，同样用换行符分割。

----begin----
{{.Body}}
----end----
	`))
)

type TranslateResult struct {
	Err  error
	Resp *TranslateResp
}

type session struct {
	dest   []string
	input  string
	respCh chan *TranslateResult
}

func newSession(dest []string, input string, respCh chan *TranslateResult) *session {
	return &session{
		dest:   dest,
		input:  input,
		respCh: respCh,
	}
}

func (s *session) fire(id string) {
	defer func() {
		if obj := recover(); obj != nil {
			err := fmt.Errorf("recovered from panic, err: %+v", obj)
			log.Errorf("%s", err)
			s.respCh <- &TranslateResult{Err: err}
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var tmpl *template.Template
	if len(s.dest) == 1 {
		tmpl = singleDestTemplate
	} else if len(s.dest) >= 2 {
		tmpl = multiDestTemplate
	}
	out := bytes.NewBuffer(nil)
	tmpl.Execute(out, struct {
		Dest []string
		Body string
	}{
		Dest: s.dest,
		Body: s.input,
	})

	ask := out.String()
	log.Debugf("ask: %s", ask)
	result := <-chatGPT.DefaultGPT().SendMsgChan(out.String(), id, ctx)
	if result.Err != nil {
		log.Errorf("request ChatGPT err: %T \"%s\"", result.Err, result.Err.Error())
		s.respCh <- &TranslateResult{Err: result.Err}
		return
	}

	translated := parseResp(result.Val)
	if translated == nil {
		log.Errorf("can't parse translate result from ChatGPT, input: %q, response: %q",
			s.input, result.Val)
		err := fmt.Errorf("can't parse translate result from ChatGPT")
		s.respCh <- &TranslateResult{Err: err}
		return
	}

	translated.Text = s.input
	s.respCh <- &TranslateResult{Resp: translated}
}

func parseResp(text string) *TranslateResp {
	var result *TranslateResp
	for _, matches := range paraPattern.FindAllStringSubmatch(text, -1) {
		result = &TranslateResp{
			From:   strings.TrimSpace(matches[1]),
			To:     strings.TrimSpace(matches[2]),
			Result: strings.Split(strings.TrimSpace(matches[3]), "\n"),
		}
	}
	return result
}
