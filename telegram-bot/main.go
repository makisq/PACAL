package main

import (
	"accses" // мой пакет
	"bytes"
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var userMeals = make(map[int64]map[string][]string)   // userID -> mealType -> dishes
var userMealTimes = make(map[int64]map[string]string) // userID -> mealType -> time
func logMessage(msg string) {
	var prettyJSON bytes.Buffer
	err := json.Indent(&prettyJSON, []byte(msg), "", "  ")
	if err != nil {
		log.Println("Ошибка форматирования JSON:", err)
		return
	}
	log.Println(prettyJSON.String())
}

// Функция для выбора случайного элемента из списка, исключая предыдущий
func menuRan(list []string, previous string) string {
	rand.Seed(time.Now().UnixNano())
	for {
		item := list[rand.Intn(len(list))]
		if item != previous {
			return item
		}
	}
}

// Функция для выбора напитка
func chooseDrink() string {
	rand.Seed(time.Now().UnixNano())
	allDrinks := []string{}
	allDrinks = append(allDrinks, accses.Brakfast1[2], accses.Brakfast1[5], accses.Brakfast1[8])
	allDrinks = append(allDrinks, accses.Breakfast2[3], accses.Breakfast2[5])
	allDrinks = append(allDrinks, accses.Lunch[2], accses.Lunch[5], accses.Lunch[8], accses.Lunch[11])
	allDrinks = append(allDrinks, accses.Snack[5], accses.Snack[11])
	allDrinks = append(allDrinks, accses.Dinner[2], accses.Dinner[5], accses.Dinner[8], accses.Dinner[11])
	return allDrinks[rand.Intn(len(allDrinks))]
}

// Функция для генерации завтрака
func generateBreakfast() []string {
	rand.Seed(time.Now().UnixNano())

	var breakfast []string
	var previousDish string

	for i := 0; i < 2; i++ {
		dish := menuRan(accses.Brakfast1, previousDish)
		breakfast = append(breakfast, dish)
		previousDish = dish
	}

	drink := chooseDrink()
	breakfast = append(breakfast, drink)

	return breakfast
}

// Функция для генерации обеда
func generateLunch() []string {
	rand.Seed(time.Now().UnixNano())

	var lunch []string
	var previousDish string

	for i := 0; i < 2; i++ {
		dish := menuRan(accses.Lunch, previousDish)
		lunch = append(lunch, dish)
		previousDish = dish
	}

	drink := chooseDrink()
	lunch = append(lunch, drink)

	return lunch
}

// Функция для генерации ужина
func generateDinner() []string {
	rand.Seed(time.Now().UnixNano())

	var dinner []string
	var previousDish string

	for i := 0; i < 2; i++ {
		dish := menuRan(accses.Dinner, previousDish)
		dinner = append(dinner, dish)
		previousDish = dish
	}

	drink := chooseDrink()
	dinner = append(dinner, drink)

	return dinner
}

// Функция для отправки сообщения
func scheduleMeal(bot *tgbotapi.BotAPI, chatID int64, mealType string, mealGenerator func() []string) {
	for {
		// Получаем время из userMealTimes
		mealTimeStr, ok := userMealTimes[chatID][mealType]
		if !ok {
			// Если время не установлено, используем значение по умолчанию
			switch mealType {
			case "завтрак":
				mealTimeStr = "08:00"
			case "обед":
				mealTimeStr = "13:00"
			case "ужин":
				mealTimeStr = "19:00"
			}
		}

		// Парсим время
		mealTime, err := time.Parse("15:04", mealTimeStr)
		if err != nil {
			log.Println("Ошибка парсинга времени:", err)
			return
		}
		log.Printf("Парсинг времени: %v", mealTime)

		loc, err := time.LoadLocation("Europe/Moscow") // Укажите нужный часовой пояс
		if err != nil {
			log.Println("Ошибка загрузки часового пояса:", err)
			return
		}

		now := time.Now().In(loc)
		nextMeal := time.Date(now.Year(), now.Month(), now.Day(), mealTime.Hour(), mealTime.Minute(), 0, 0, loc)

		// Если время уже прошло сегодня, планируем на следующий день
		if now.After(nextMeal) {
			nextMeal = nextMeal.Add(24 * time.Hour)
		}

		// Ожидаем до времени следующего приёма пищи
		time.Sleep(time.Until(nextMeal))

		// Генерируем рацион
		meal := mealGenerator()

		// Формируем сообщение
		msgText := "Ваше меню на " + mealType + ":\n" + strings.Join(meal, "\n")

		// Отправляем сообщение
		msg := tgbotapi.NewMessage(chatID, msgText)
		_, err = bot.Send(msg)
		if err != nil {
			log.Println("Ошибка при отправке сообщения:", err)
		}
	}
}

func main() {
	var userChatID int64 // Объявляем переменную для хранения chatID

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Println("Токен не обнаружен")
		log.Panic("Токен не обнаружен")
	}
	log.Printf("Токен: %s", botToken) // Логируем токен для проверки

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Println("Ошибка при создании бота:", err)
		log.Panic("Ошибка при создании бота: ", err)
	}

	bot.Debug = true
	log.Printf("Бот авторизован как %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

			var msg tgbotapi.MessageConfig

			switch update.Message.Command() {
			case "start":
				userChatID = update.Message.Chat.ID // Сохраняем chatID
				msg = tgbotapi.NewMessage(userChatID, "Добро пожаловать! Я ваш бот для планирования рациона при панкреатите. Для вывода команд, используете /help")

				// Запускаем горутины для отправки еды по расписанию
				go scheduleMeal(bot, userChatID, "завтрак", generateBreakfast)
				go scheduleMeal(bot, userChatID, "обед", generateLunch)
				go scheduleMeal(bot, userChatID, "ужин", generateDinner)
			case "help":
				helpText := `Я могу помочь вам с командами:
			/start - Запустить бота
			/help - Показать это сообщение
			/menu - Показать меню на сегодня
			/all_breakfast - Показать все блюда на завтрак
			/all_lunch - Показать все блюда на обед
			/all_dinner - Показать все блюда на ужин
			/set_time - выбрать время  `
				msg = tgbotapi.NewMessage(userChatID, helpText)
			case "menu":
				meal := generateBreakfast()
				msg = tgbotapi.NewMessage(userChatID, "Ваше меню на сегодня:\n"+meal[0]+"\n"+meal[1]+"\n"+meal[2])
			case "all_breakfast":
				msg = tgbotapi.NewMessage(userChatID, "Все блюда на завтрак:\n"+strings.Join(accses.Brakfast1, "\n"))
			case "all_lunch":
				msg = tgbotapi.NewMessage(userChatID, "Все блюда на обед:\n"+strings.Join(accses.Lunch, "\n"))
			case "all_dinner":
				msg = tgbotapi.NewMessage(userChatID, "Все блюда на ужин:\n"+strings.Join(accses.Dinner, "\n"))
			case "set_time":
				parts := strings.Fields(update.Message.Text)
				if len(parts) < 3 {
					msg = tgbotapi.NewMessage(userChatID, "Используйте формат: /set_time завтрак 10:00")
				} else {
					mealType := parts[1]
					mealTime := ""

					for _, part := range parts[2:] {
						_, err := time.Parse("15:04", part)
						if err == nil {
							mealTime = part
							break
						}
					}

					if mealTime == "" {
						msg = tgbotapi.NewMessage(userChatID, "Некорректный формат времени. Используйте формат HH:MM, например, 10:00")
					} else {
						// Сохраняем время
						if _, ok := userMealTimes[userChatID]; !ok {
							userMealTimes[userChatID] = make(map[string]string)
						}
						userMealTimes[userChatID][mealType] = mealTime
						msg = tgbotapi.NewMessage(userChatID, "Время для "+mealType+" установлено на "+mealTime)
					}
				}
			default:
				msg = tgbotapi.NewMessage(userChatID, "Я не знаю такой команды")
			}

			_, err := bot.Send(msg)
			if err != nil {
				log.Println("Ошибка при отправке сообщения:", err)
			}
		}
	}
}
