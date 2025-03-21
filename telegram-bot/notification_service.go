package main

import (
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func ScheduleMeal(bot *tgbotapi.BotAPI, chatID int64, mealType string, mealGenerator func() []string) {
	for {
		userData := getUserData(chatID)
		mealTimeStr, ok := userData.MealTimes[mealType]
		if !ok {
			switch mealType {
			case "завтрак":
				mealTimeStr = "08:00"
			case "обед":
				mealTimeStr = "13:00"
			case "ужин":
				mealTimeStr = "19:00"
			}
		}

		mealTime, _ := time.Parse("15:04", mealTimeStr)
		loc, _ := time.LoadLocation("Europe/Moscow")
		now := time.Now().In(loc)
		nextMeal := time.Date(now.Year(), now.Month(), now.Day(), mealTime.Hour(), mealTime.Minute(), 0, 0, loc)

		if now.After(nextMeal) {
			nextMeal = nextMeal.Add(24 * time.Hour)
		}

		time.Sleep(time.Until(nextMeal))

		meal := mealGenerator()
		msgText := "Ваше меню на " + mealType + ":\n" + strings.Join(meal, "\n")
		msg := tgbotapi.NewMessage(chatID, msgText)
		_, err := bot.Send(msg)
		if err != nil {
			log.Println("Ошибка при отправке сообщения:", err)
		}
	}
}
