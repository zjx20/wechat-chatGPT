package translate

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	maxConcurrent = 1
)

var sem = make(chan string, maxConcurrent)

func init() {
	for i := 0; i < cap(sem); i++ {
		sem <- fmt.Sprintf("translate_%d", i)
	}
}

func goFire(s *session) {
	id := <-sem
	defer func() {
		sem <- id
	}()
	s.fire(id)
}

func Translate(req *TranslateReq, ch chan *TranslateResult) {
	req.Text = strings.TrimSpace(req.Text)
	if len(req.Destination) == 0 || req.Text == "" {
		log.Errorf("bad translate req: %+v", req)
		return
	}
	s := newSession(req.Destination, req.Text, ch)
	go goFire(s)
}
