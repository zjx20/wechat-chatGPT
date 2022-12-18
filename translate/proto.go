package translate

import (
	"fmt"
	"net/http"
	"strings"
)

// https://hcfy.app/docs/services/custom-api

type TranslateReq struct {
	Name        string   `json:"name"`
	Text        string   `json:"text"`
	Destination []string `json:"destination"`
	Source      string   `json:"source"`
}

func (req *TranslateReq) Bind(r *http.Request) error {
	if len(req.Destination) == 0 {
		return fmt.Errorf("destination should not be empty")
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		return fmt.Errorf("text should not be empty")
	}
	return nil
}

type TranslateResp struct {
	Text   string   `json:"text"`
	From   string   `json:"from"`
	To     string   `json:"to"`
	Result []string `json:"result"`
}
