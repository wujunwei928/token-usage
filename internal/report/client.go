package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wujunwei928/token-usage/internal/config"
)

// firstNonEmptyEnv returns the first non-empty value among the names:
// TOKEN_USAGE_* wins over the legacy CCUSAGE_* names (ADR 0008).
func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return ""
}

// Endpoint config resolution order: flag > env (TOKEN_USAGE_REPORT_SERVER /
// TOKEN_USAGE_REPORT_TOKEN, falling back to CCUSAGE_REPORT_SERVER /
// CCUSAGE_REPORT_TOKEN) > token-usage config keys reportServer / reportToken.

// ResolveEndpoint applies the precedence chain. args are the raw CLI args so
// config discovery sees --config like every other command.
func ResolveEndpoint(args []string, flagServer, flagToken string) (server, token string, err error) {
	server = strings.TrimRight(strings.TrimSpace(flagServer), "/")
	if server == "" {
		server = strings.TrimRight(firstNonEmptyEnv("TOKEN_USAGE_REPORT_SERVER", "CCUSAGE_REPORT_SERVER"), "/")
	}
	token = strings.TrimSpace(flagToken)
	if token == "" {
		token = firstNonEmptyEnv("TOKEN_USAGE_REPORT_TOKEN", "CCUSAGE_REPORT_TOKEN")
	}
	if server == "" || token == "" {
		root := config.LoadConfigValue(config.ScanConfigPath(args))
		if root != nil {
			if server == "" {
				if v, ok := root["reportServer"].(string); ok {
					server = strings.TrimRight(strings.TrimSpace(v), "/")
				}
			}
			if token == "" {
				if v, ok := root["reportToken"].(string); ok {
					token = strings.TrimSpace(v)
				}
			}
		}
	}
	if server == "" {
		return "", "", ErrNoServer
	}
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "http://" + server
	}
	if token == "" {
		return "", "", ErrNoToken
	}
	return server, token, nil
}

// SendResult is the server's response to one Report Snapshot.
type SendResult struct {
	Accepted    bool   `json:"accepted"`
	DeviceCount int    `json:"deviceCount"`
	DayTokens   string `json:"dayTokens"`
	DayDate     string `json:"dayDate"`
	Days        int    `json:"days"`
}

// Client errors distinguish network failures from server rejections so the
// CLI can pick the right exit code and advice.
type ClientError struct {
	Kind    string // "network" | "server"
	Status  int
	Code    string
	Message string
}

func (e *ClientError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("server rejected report (HTTP %d, %s): %s", e.Status, e.Code, e.Message)
	}
	return e.Message
}

// Send ships one snapshot with a short timeout.
func Send(server, token string, snapshot *Snapshot, timeout time.Duration) (*SendResult, error) {
	return SendMany(server, token, []*Snapshot{snapshot}, timeout)
}

// SendMany ships one or more daily snapshots in a single request (the
// backfill shape); a single day uses the steady-state shape unchanged.
func SendMany(server, token string, snapshots []*Snapshot, timeout time.Duration) (*SendResult, error) {
	var payload any
	if len(snapshots) == 1 {
		payload = snapshots[0]
	} else {
		type dayPayload struct {
			Date  string     `json:"date"`
			Hours []HourCell `json:"hours"`
		}
		days := make([]dayPayload, 0, len(snapshots))
		for _, s := range snapshots {
			days = append(days, dayPayload{Date: s.Date, Hours: s.Hours})
		}
		payload = struct {
			DeviceID    string `json:"deviceId"`
			DeviceLabel string `json:"deviceLabel"`
			Timezone    string `json:"timezone"`
			GeneratedAt string `json:"generatedAt"`
			Days        any    `json:"days"`
		}{
			DeviceID:    snapshots[0].DeviceID,
			DeviceLabel: snapshots[0].DeviceLabel,
			Timezone:    snapshots[0].Timezone,
			GeneratedAt: snapshots[0].GeneratedAt,
			Days:        days,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, server+"/v1/report", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, &ClientError{Kind: "network", Message: "cannot reach report server " + server + ": " + err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var rej struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		json.Unmarshal(raw, &rej)
		if rej.Message == "" {
			rej.Message = strings.TrimSpace(string(raw))
		}
		return nil, &ClientError{Kind: "server", Status: resp.StatusCode, Code: rej.Code, Message: rej.Message}
	}
	var result SendResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, &ClientError{Kind: "server", Status: resp.StatusCode, Message: "malformed server response"}
	}
	return &result, nil
}
