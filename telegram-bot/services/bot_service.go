package services

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotService struct {
	Bot              *tgbotapi.BotAPI
	MenuService      *MenuService
	SchedulerService *SchedulerService
}

func (bs *BotService) SendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)

	// Добавляем задержку для защиты от флуда
	time.Sleep(300 * time.Millisecond)

	if _, err := bs.Bot.Send(msg); err != nil {
		log.Printf("Ошибка отправки сообщения: %v", err)
	}
}

func NewBotService(token string, menuService *MenuService, schedulerService *SchedulerService) (*BotService, error) {
	if token == "" {
		return nil, errors.New("токен бота не может быть пустым")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, errors.New("ошибка создания бота: " + err.Error())
	}

	bot.Debug = true // Включение режима отладки
	log.Printf("Бот авторизован как %s", bot.Self.UserName)

	return &BotService{
		Bot:              bot,
		MenuService:      menuService,
		SchedulerService: schedulerService,
	}, nil
}

func (bs *BotService) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bs.Bot.GetUpdatesChan(u)

	log.Println("Ожидание сообщений...")
	for update := range updates {
		if update.Message != nil {
			bs.handleMessage(update.Message)
		}
	}
}

func (bs *BotService) handleMessage(msg *tgbotapi.Message) {
	var response string

	switch msg.Command() {
	case "start":
		if bs.SchedulerService != nil {
			bs.SchedulerService.StartScheduling(msg.Chat.ID)
		}
		response = `Добро пожаловать! Я ваш бот для планирования рациона.
        
Доступные команды:
/menu - Показать меню на сегодня
/settime - Установить время приема пищи
/times - Показать текущее расписание
/help - Справка`

	case "menu":
		if bs.MenuService != nil {
			response = bs.MenuService.GenerateDailyMenu(msg.Chat.ID)
		} else {
			response = "Сервис меню временно недоступен"
		}

	case "settime":
		response = `Установите время приема пищи:
Пример: /settime breakfast 08:30
Доступные типы: breakfast, lunch, dinner`

	case "times":
		if bs.SchedulerService != nil && bs.MenuService != nil {
			breakfast, _ := bs.MenuService.GetMealTime(msg.Chat.ID, "breakfast")
			lunch, _ := bs.MenuService.GetMealTime(msg.Chat.ID, "lunch")
			dinner, _ := bs.MenuService.GetMealTime(msg.Chat.ID, "dinner")

			now := time.Now()
			var builder strings.Builder
			builder.WriteString("⏰ Ваше текущее расписание:\n")

			// Функция для расчета времени до следующего приема пищи
			calculateNext := func(mealTimeStr string, mealName string) {
				mealTime, _ := time.Parse("15:04", mealTimeStr)
				next := time.Date(now.Year(), now.Month(), now.Day(),
					mealTime.Hour(), mealTime.Minute(), 0, 0, now.Location())

				if next.Before(now) {
					next = next.Add(24 * time.Hour)
					builder.WriteString(fmt.Sprintf("%s: %s (завтра, через %v)\n",
						mealName, mealTimeStr, next.Sub(now).Round(time.Minute)))
				} else {
					builder.WriteString(fmt.Sprintf("%s: %s (сегодня, через %v)\n",
						mealName, mealTimeStr, next.Sub(now).Round(time.Minute)))
				}
			}

			calculateNext(breakfast, "🍳 Завтрак")
			calculateNext(lunch, "🍲 Обед")
			calculateNext(dinner, "🍽 Ужин")

			response = builder.String()
		} else {
			response = "Сервис расписания временно недоступен"
		}

	case "help":
		response = `Справка по командам:
/start - Начать работу
/menu - Меню на сегодня
/settime - Установить время приема пищи
/times - Показать текущее расписание
/help - Эта справка`

	default:
		parts := strings.Fields(msg.Text)
		if len(parts) == 2 {
			mealType := strings.ToLower(parts[0])
			timeStr := parts[1]

			validMealTypes := map[string]bool{"breakfast": true, "lunch": true, "dinner": true}

			if validMealTypes[mealType] {
				notice, err := bs.MenuService.SetMealTime(msg.Chat.ID, mealType, timeStr)
				if err != nil {
					response = "❌ " + err.Error()
				} else {
					// Получаем обновленное расписание
					breakfast, _ := bs.MenuService.GetMealTime(msg.Chat.ID, "breakfast")
					lunch, _ := bs.MenuService.GetMealTime(msg.Chat.ID, "lunch")
					dinner, _ := bs.MenuService.GetMealTime(msg.Chat.ID, "dinner")

					response = fmt.Sprintf(`✅ Время для %s установлено на %s%s

Ваше новое расписание:
🍳 Завтрак: %s
🍲 Обед: %s
🍽 Ужин: %s`,
						mealType, timeStr, notice, breakfast, lunch, dinner)
				}
			} else {
				response = "❌ Неправильный тип приема пищи. Доступные: breakfast, lunch, dinner"
			}
		} else {
			response = "Неизвестная команда. Используйте /help для справки"
		}
	}

	bs.SendMessage(msg.Chat.ID, response)
}
