package services

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type SchedulerService struct {
	userMealTimes map[int64]map[string]string // userID -> mealType -> time
	bot           *tgbotapi.BotAPI
	menuService   *MenuService
}

func NewSchedulerService(bot *tgbotapi.BotAPI, menuService *MenuService) *SchedulerService {
	return &SchedulerService{
		userMealTimes: make(map[int64]map[string]string),
		bot:           bot,
		menuService:   menuService,
	}
}

func (ss *SchedulerService) StartScheduling(userID int64) {
	if ss.menuService == nil {
		log.Printf("MenuService is nil for user %d", userID)
		return
	}

	// Проверка методов через reflect
	requiredMethods := []string{"GenerateBreakfast", "GenerateLunch", "GenerateDinner"}
	for _, method := range requiredMethods {
		if _, ok := reflect.TypeOf(ss.menuService).MethodByName(method); !ok {
			log.Printf("Method %s not found in MenuService", method)
			return
		}
	}

	// Запуск планировщиков для каждого приема пищи
	go ss.scheduleMeal(userID, "breakfast", ss.menuService.GenerateBreakfast)
	go ss.scheduleMeal(userID, "lunch", ss.menuService.GenerateLunch)
	go ss.scheduleMeal(userID, "dinner", ss.menuService.GenerateDinner)
}

func (ss *SchedulerService) SetMealTime(userID int64, mealType, timeStr string) error {
	mealTime, err := time.Parse("15:04", timeStr)
	if err != nil {
		return fmt.Errorf("неверный формат времени. Используйте HH:MM")
	}
	now := time.Now().In(mealTime.Location())
	if mealTime.Before(now.Add(-time.Minute)) {
		return fmt.Errorf("указанное время (%s) уже прошло. Укажите будущее время", timeStr)
	}

	if _, ok := ss.userMealTimes[userID]; !ok {
		ss.userMealTimes[userID] = make(map[string]string)
	}

	ss.userMealTimes[userID][mealType] = timeStr
	return nil
}

func (ss *SchedulerService) GetMealTime(userID int64, mealType string) string {
	if times, ok := ss.userMealTimes[userID]; ok {
		if t, ok := times[mealType]; ok {
			return t
		}
	}
	// Возвращаем время по умолчанию
	switch mealType {
	case "breakfast":
		return "08:00"
	case "lunch":
		return "13:00"
	case "dinner":
		return "19:00"
	default:
		return "12:00"
	}
}

func (ss *SchedulerService) scheduleMeal(userID int64, mealType string, mealGenerator func() []string) {
	for {
		mealTimeStr := ss.GetMealTime(userID, mealType)
		mealTime, _ := time.Parse("15:04", mealTimeStr)

		loc, _ := time.LoadLocation("Europe/Moscow")
		now := time.Now().In(loc)
		nextMeal := time.Date(now.Year(), now.Month(), now.Day(),
			mealTime.Hour(), mealTime.Minute(), 0, 0, loc)

		if now.After(nextMeal) {
			nextMeal = nextMeal.Add(24 * time.Hour)
		}

		sleepDuration := time.Until(nextMeal)
		log.Printf("User %d: next %s at %s (in %v)",
			userID, mealType, nextMeal.Format("15:04"), sleepDuration)

		time.Sleep(sleepDuration)

		// Генерация и отправка меню
		meal := mealGenerator()
		msgText := fmt.Sprintf("🍽 Ваше меню на %s:\n%s",
			mealType, strings.Join(meal, "\n"))
		msg := tgbotapi.NewMessage(userID, msgText)

		if _, err := ss.bot.Send(msg); err != nil {
			log.Printf("Ошибка отправки меню пользователю %d: %v", userID, err)
		}
	}
}
