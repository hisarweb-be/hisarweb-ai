package service

// An escalation card on the Bureau V2 board (Paperclip): the admin agent
// files a larger problem there instead of trying to solve it in the chat.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	HwBureauURLEnv       = "HW_BUREAU_URL"
	HwBureauCompanyEnv   = "HW_BUREAU_COMPANY_ID"
	HwBureauTokenEnv     = "HW_BUREAU_TOKEN"
	hwBureauCardMaxRunes = 4000
)

// HwBureauCardConfig is where an escalation card is created. Token is
// optional; the board in local_trusted mode takes header-less calls.
type HwBureauCardConfig struct {
	BaseURL   string
	CompanyID string
	Token     string
	Client    *http.Client
}

func HwBureauCardConfigFromEnv() HwBureauCardConfig {
	return HwBureauCardConfig{
		BaseURL:   strings.TrimSpace(os.Getenv(HwBureauURLEnv)),
		CompanyID: strings.TrimSpace(os.Getenv(HwBureauCompanyEnv)),
		Token:     strings.TrimSpace(os.Getenv(HwBureauTokenEnv)),
	}
}

type hwBureauIssueRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Status      string `json:"status"`
}

type hwBureauIssueResponse struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"`
}

// HwBureauCreateCard creates one card on the board and returns its identifier.
func HwBureauCreateCard(ctx context.Context, config HwBureauCardConfig, title, summary, page string) (string, error) {
	if config.BaseURL == "" || config.CompanyID == "" {
		return "", errors.New("the bureau board is not connected")
	}
	title = strings.TrimSpace(title)
	summary = strings.TrimSpace(summary)
	if title == "" || summary == "" {
		return "", errors.New("title and summary are required")
	}
	description := "Gemeld via het admin-paneel van api.hisarweb.be (admin-agent).\n"
	if page = strings.TrimSpace(page); page != "" {
		description += "Pagina: " + page + "\n"
	}
	description += "\n" + summary
	if runes := []rune(description); len(runes) > hwBureauCardMaxRunes {
		description = string(runes[:hwBureauCardMaxRunes])
	}
	body, err := common.Marshal(hwBureauIssueRequest{
		Title: "Admin-agent: " + title, Description: description, Priority: "medium", Status: "todo",
	})
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(config.BaseURL, "/") + "/api/companies/" + config.CompanyID + "/issues"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if config.Token != "" {
		req.Header.Set("Authorization", "Bearer "+config.Token)
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("the bureau board could not be reached: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("the bureau board returned status %d", resp.StatusCode)
	}
	var issue hwBureauIssueResponse
	if err := common.DecodeJson(resp.Body, &issue); err != nil {
		return "", fmt.Errorf("the bureau board returned an unreadable body: %w", err)
	}
	if issue.Identifier != "" {
		return issue.Identifier, nil
	}
	return issue.ID, nil
}
