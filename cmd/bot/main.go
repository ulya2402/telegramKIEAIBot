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
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if err := core.LoadRegistry("models.json"); err != nil {
		log.Fatalf("Critical Error: %v", err)
	}
	fmt.Println("AI Models loaded from models.json")

	db, err := database.NewSQLiteDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	kieClient := api.NewKieClient(cfg.KieAPIKey)
	loc := i18n.NewLocalizer(cfg.DefaultLang)

	telegramBot := bot.NewBot(cfg.TelegramToken, db, kieClient, loc)

	fmt.Println("System initialized. Bot is now running...")
	telegramBot.Start()
}