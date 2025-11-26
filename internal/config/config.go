package config

import (
	"bufio"
	"kieAITelegram/internal/models"
	"log"
	"os"
	"strings"
)

func LoadConfig() (*models.Config, error) {
	file, err := os.Open(".env")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := &models.Config{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "TELEGRAM_BOT_TOKEN":
			config.TelegramToken = value
		case "KIE_API_KEY":
			config.KieAPIKey = value
		case "DB_PATH":
			config.DBPath = value
		case "DEFAULT_LANG":
			config.DefaultLang = value
		}
	}

	if config.TelegramToken == "" || config.KieAPIKey == "" {
		log.Println("Error: Missing critical environment variables")
	}

	return config, scanner.Err()
}