package bot

import (
	"log"
	"os"
	"time"

	tele "gopkg.in/telebot.v3"
)

func StartTelegramBot() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Println("TELEGRAM_BOT_TOKEN not set, skipping Telegram bot startup")
		return
	}
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}
	b, err := tele.NewBot(pref)
	if err != nil {
		log.Fatalf("failed to create Telegram bot: %v", err)
	}
	b.Handle("/ping", func(c tele.Context) error {
		return c.Send("pong")
	})
	log.Println("Telegram bot started")
	go b.Start()
}
