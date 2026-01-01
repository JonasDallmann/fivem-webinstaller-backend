package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type DiscordLogger struct {
	WebhookURL string
}

func NewDiscordLogger() *DiscordLogger {
	return &DiscordLogger{
		WebhookURL: os.Getenv("DISCORD_WEBHOOK_URL"),
	}
}

func (l *DiscordLogger) LogError(context, message string, details string) {
	if l.WebhookURL == "" {
		fmt.Println("Discord Webhook URL not set. Logging to console only.")
		fmt.Printf("[%s] ERROR: %s - %s\n", context, message, details)
		return
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       "FiveM Installer Error",
				"description": fmt.Sprintf("**Context:** %s\n**Message:** %s", context, message),
				"color":       15158332, // Red
				"fields": []map[string]interface{}{
					{
						"name":  "Details",
						"value": "```\n" + truncateString(details, 1000) + "\n```",
					},
				},
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Failed to marshal discord payload: %v\n", err)
		return
	}

	resp, err := http.Post(l.WebhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		fmt.Printf("Failed to send log to Discord: %v\n", err)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Printf("Failed to close response body: %v\n", err)
		}
	}(resp.Body)

	if resp.StatusCode != 204 && resp.StatusCode != 200 {
		fmt.Printf("Discord Webhook returned status: %d\n", resp.StatusCode)
	}
}

func (l *DiscordLogger) LogInfo(context, message string) {
	if l.WebhookURL == "" {
		fmt.Printf("[%s] INFO: %s\n", context, message)
		return
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       "FiveM Installer Info",
				"description": fmt.Sprintf("**Context:** %s\n**Message:** %s", context, message),
				"color":       3066993, // Green
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
	}

	jsonPayload, _ := json.Marshal(payload)
	_, err := http.Post(l.WebhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return
	}
}

func truncateString(str string, num int) string {
	if len(str) <= num {
		return str
	}
	if num > 3 {
		return str[:num-3] + "..."
	}
	return str[:num]
}
