package services

import (
	"accses"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"
)

type MenuService struct {
	randSource     *rand.Rand
	previousDishes map[string]string
	userMealTimes  map[int64]map[string]string
}

func NewMenuService() *MenuService {
	return &MenuService{
		randSource:     rand.New(rand.NewSource(time.Now().UnixNano())),
		previousDishes: make(map[string]string),
		userMealTimes:  make(map[int64]map[string]string),
	}
}

// Установка времени приема пищи для пользователя
func (ms *MenuService) SetMealTime(userID int64, mealType, timeStr string) (string, error) {
	// Парсим время с валидацией формата
	mealTime, err := time.Parse("15:04", timeStr)
	if err != nil {
		return "", fmt.Errorf("неверный формат времени. Используйте HH:MM")
	}

	// Проверка допустимых значений времени
	if mealTime.Hour() > 23 || mealTime.Minute() > 59 {
		return "", fmt.Errorf("несуществующее время. Максимум 23:59")
	}

	// Текущее время
	now := time.Now()
	currentTime := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, now.Location())
	scheduledTime := time.Date(now.Year(), now.Month(), now.Day(), mealTime.Hour(), mealTime.Minute(), 0, 0, now.Location())

	// Проверка логичности времени для типа приема пищи
	switch mealType {
	case "breakfast":
		if mealTime.Hour() < 3 || mealTime.Hour() > 11 {
			return "", fmt.Errorf("странное время для завтрака (%s). Обычно между 05:00 и 11:00", timeStr)
		}
	case "lunch":
		if mealTime.Hour() < 11 || mealTime.Hour() > 16 {
			return "", fmt.Errorf("странное время для обеда (%s). Обычно между 12:00 и 15:00", timeStr)
		}
	case "dinner":
		if mealTime.Hour() < 16 || mealTime.Hour() > 23 {
			return "", fmt.Errorf("странное время для ужина (%s). Обычно между 17:00 и 22:00", timeStr)
		}
	default:
		return "", fmt.Errorf("неизвестный тип приема пищи: %s", mealType)
	}

	// Проверяем, прошло ли время сегодня
	var notice string
	if scheduledTime.Before(currentTime) {
		scheduledTime = scheduledTime.Add(24 * time.Hour)
		notice = fmt.Sprintf("\n⚠ Заметка: Указанное время уже прошло сегодня, поэтому уведомление придет завтра в %s", timeStr)
	}

	// Инициализация map если нужно
	if ms.userMealTimes[userID] == nil {
		ms.userMealTimes[userID] = make(map[string]string)
	}

	// Сохраняем оригинальное время (без переноса)
	ms.userMealTimes[userID][mealType] = timeStr

	// Логирование
	log.Printf("Пользователь %d установил время %s на %s", userID, mealType, timeStr)

	// Возвращаем время и уведомление о переносе (если было)
	return notice, nil
}

// Получение установленного времени приема пищи
func (ms *MenuService) GetMealTime(userID int64, mealType string) (string, error) {
	// Проверка допустимых типов приема пищи
	validMealTypes := map[string]bool{
		"breakfast": true,
		"lunch":     true,
		"dinner":    true,
	}

	if !validMealTypes[mealType] {
		return "", fmt.Errorf("неподдерживаемый тип приема пищи: %s", mealType)
	}

	// Проверка наличия пользователя и времени
	if userTimes, ok := ms.userMealTimes[userID]; ok {
		if timeStr, ok := userTimes[mealType]; ok {
			// Дополнительная валидация сохраненного времени
			if _, err := time.Parse("15:04", timeStr); err != nil {
				return "", fmt.Errorf("некорректное время в настройках: %s", timeStr)
			}
			return timeStr, nil
		}
	}

	// Возвращаем время по умолчанию с предупреждением в лог
	defaultTimes := map[string]string{
		"breakfast": "08:00",
		"lunch":     "13:00",
		"dinner":    "19:00",
	}

	defaultTime := defaultTimes[mealType]
	log.Printf("Использовано время по умолчанию для пользователя %d, прием пищи: %s - %s",
		userID, mealType, defaultTime)

	return defaultTime, nil
}

func (ms *MenuService) GetNextMealTime(userID int64, mealType string) (time.Time, error) {
	timeStr, err := ms.GetMealTime(userID, mealType)
	if err != nil {
		return time.Time{}, err
	}

	mealTime, _ := time.Parse("15:04", timeStr)
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(),
		mealTime.Hour(), mealTime.Minute(), 0, 0, now.Location())

	if next.Before(now) {
		next = next.Add(24 * time.Hour)
	}

	return next, nil
}

// Генерация полного меню на день
func (ms *MenuService) GenerateDailyMenu(userID int64) string {
	var builder strings.Builder

	// Получаем все времена приема пищи
	breakfastTime, _ := ms.GetMealTime(userID, "breakfast")
	lunchTime, _ := ms.GetMealTime(userID, "lunch")
	dinnerTime, _ := ms.GetMealTime(userID, "dinner")

	builder.WriteString("🍽 Ваше меню на сегодня:\n\n")

	// Завтрак
	builder.WriteString("🌅 Завтрак (" + breakfastTime + "):\n")
	builder.WriteString(strings.Join(ms.GenerateBreakfast(), "\n"))

	// Обед
	builder.WriteString("\n\n☀ Обед (" + lunchTime + "):\n")
	builder.WriteString(strings.Join(ms.GenerateLunch(), "\n"))

	// Ужин
	builder.WriteString("\n\n🌙 Ужин (" + dinnerTime + "):\n")
	builder.WriteString(strings.Join(ms.GenerateDinner(), "\n"))

	// Время приема пищи
	builder.WriteString("\n\n⏰ Расписание:\n")
	builder.WriteString(fmt.Sprintf("Завтрак: %s\nОбед: %s\nУжин: %s",
		breakfastTime,
		lunchTime,
		dinnerTime))

	return builder.String()
}

func (ms *MenuService) getRandomDish(list []string, mealType string) string {
	previous := ms.previousDishes[mealType]

	for i := 0; i < 100; i++ { // Защита от бесконечного цикла
		item := list[ms.randSource.Intn(len(list))]
		if item != previous {
			ms.previousDishes[mealType] = item
			return item
		}
	}
	return list[0] // fallback
}

func (ms *MenuService) getRandomDrink() string {
	drinkLists := [][]string{
		accses.Brakfast1[2:9:9],  // Берем только напитки из завтрака 1
		accses.Breakfast2[3:6:6], // Напитки из завтрака 2
		accses.Lunch[2:12:12],    // Напитки из обеда
		accses.Dinner[2:12:12],   // Напитки из ужина
	}

	// Объединяем все напитки в один список
	var allDrinks []string
	for _, drinks := range drinkLists {
		allDrinks = append(allDrinks, drinks...)
	}

	if len(allDrinks) == 0 {
		return "Чай" // fallback
	}
	return allDrinks[ms.randSource.Intn(len(allDrinks))]
}

func (ms *MenuService) generateMeal(courseGroups [][]string, count int, mealType string) []string {
	var result []string
	var liquidItems []string
	liquidKeywords := []string{"чай", "компот", "кисель", "кефир", "ряженка"}

	// Собираем все возможные жидкости
	for _, group := range courseGroups {
		for _, item := range group {
			for _, keyword := range liquidKeywords {
				if strings.Contains(strings.ToLower(item), keyword) {
					liquidItems = append(liquidItems, item)
					break
				}
			}
		}
	}

	usedLiquids := make(map[string]bool)

	for i := 0; i < len(courseGroups) && len(result) < count; i++ {
		group := courseGroups[i]
		if len(group) == 0 {
			continue
		}

		maxAttempts := 10
		for attempts := 0; attempts < maxAttempts; attempts++ {
			item := group[rand.Intn(len(group))]

			// Проверяем, является ли блюдо жидкостью
			isLiquid := false
			for _, keyword := range liquidKeywords {
				if strings.Contains(strings.ToLower(item), keyword) {
					isLiquid = true
					break
				}
			}

			// Если это жидкость и она уже использована, пропускаем
			if isLiquid && usedLiquids[item] {
				continue
			}

			// Если это жидкость, отмечаем ее как использованную
			if isLiquid {
				usedLiquids[item] = true
			}

			// Проверяем, что в результате нет дублирования жидкостей
			duplicateLiquid := false
			if isLiquid {
				for _, resItem := range result {
					for _, keyword := range liquidKeywords {
						if strings.Contains(strings.ToLower(resItem), keyword) {
							duplicateLiquid = true
							break
						}
					}
					if duplicateLiquid {
						break
					}
				}
			}

			if !duplicateLiquid {
				result = append(result, item)
				break
			}
		}
	}

	// Если не удалось набрать нужное количество блюд, попробуем без ограничений
	if len(result) < count {
		for i := 0; i < len(courseGroups) && len(result) < count; i++ {
			group := courseGroups[i]
			if len(group) == 0 {
				continue
			}

			item := group[rand.Intn(len(group))]
			// Проверяем, нет ли уже этого блюда в результате
			exists := false
			for _, resItem := range result {
				if resItem == item {
					exists = true
					break
				}
			}
			if !exists {
				result = append(result, item)
			}
		}
	}

	return result
}

func (ms *MenuService) GenerateLunch() []string {
	mainCourses := [][]string{
		accses.Lunch[:6],   // Первые блюда
		accses.Lunch[6:12], // Вторые блюда
	}
	return ms.generateMeal(mainCourses, 2, "lunch")
}

func (ms *MenuService) GenerateBreakfast() []string {
	mainCourses := [][]string{
		accses.Brakfast1,  // Основные блюда из первого завтрака
		accses.Breakfast2, // Основные блюда из второго завтрака
	}
	return ms.generateMeal(mainCourses, 2, "breakfast")
}

func (ms *MenuService) GenerateDinner() []string {
	mainCourses := [][]string{
		accses.Dinner[:6],   // Основные блюда
		accses.Dinner[6:12], // Дополнительные блюда
	}
	return ms.generateMeal(mainCourses, 2, "dinner")
}
