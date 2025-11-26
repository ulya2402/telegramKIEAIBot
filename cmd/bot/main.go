package main

import (
	"fmt"
	"kieAITelegram/internal/api"
	"kieAITelegram/internal/bot"
	"kieAITelegram/internal/config"
	"kieAITelegram/internal/core"
	"kieAITelegram/internal/database"
	"kieAITelegram/internal/i18n"
	"log"
)

func main() {
	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// 2. Load AI Models from JSON
	if err := core.LoadRegistry("models.json"); err != nil {
		log.Fatalf("Critical Error: %v", err)
	}
	fmt.Println("AI Models loaded from models.json")

	// 3. Init Database
	db, err := database.NewSQLiteDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// 4. Init API Client & Localization
	kieClient := api.NewKieClient(cfg.KieAPIKey)
	loc := i18n.NewLocalizer(cfg.DefaultLang)

	// 5. Start Bot
	telegramBot := bot.NewBot(cfg.TelegramToken, db, kieClient, loc)

	fmt.Println("System initialized. Bot is now running...")
	telegramBot.Start()
}