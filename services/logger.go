package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type DiscordLogger struct {
	WebhookURL string
}

func NewDiscordLogger() *DiscordLogger {
	url := os.Getenv("DISCORD_WEBHOOK_URL")
	if url == "" {
		fmt.Println("WARNUNG: DISCORD_WEBHOOK_URL ist nicht gesetzt!")
	}
	return &DiscordLogger{WebhookURL: url}
}

func (l *DiscordLogger) LogInfo(section, msg string) {
	l.sendEmbed(section, msg, 5763719)
}

func (l *DiscordLogger) LogError(section, msg, err string) {
	fullMsg := fmt.Sprintf("**Message:** %s\n\n**Error:**\n```\n%s\n```", msg, err)
	l.sendEmbed(section, fullMsg, 15548997)
}

func (l *DiscordLogger) sendEmbed(title, description string, color int) {
	if l.WebhookURL == "" {
		fmt.Printf("[LOG %s] %s\n", title, description)
		return
	}

	germanTime := time.Now().Format("02.01.2006 15:04:05")

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       title,
				"description": description,
				"color":       color,
				"footer": map[string]interface{}{
					"text": "📅 " + germanTime + " Uhr",
				},
			},
		},
	}

	jsonPayload, _ := json.Marshal(payload)
	resp, err := http.Post(l.WebhookURL, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		fmt.Println("Discord Webhook Error:", err)
		return
	}
	defer resp.Body.Close()
}