package main

import (
	"log"
	"os"
	"path/filepath"

	"services"

	"github.com/joho/godotenv"
)

func main() {
	// Загрузка .env файла
	envPath := filepath.Join(".", ".env")
	if err := godotenv.Load(envPath); err != nil {
		log.Fatalf("Ошибка загрузки .env файла: %v", err)
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("Токен не найден. Убедитесь, что переменная TELEGRAM_BOT_TOKEN установлена в .env файле")
	}

	// Инициализация сервисов
	menuService := services.NewMenuService()

	// Создание сервиса бота
	botService, err := services.NewBotService(token, menuService, nil)
	if err != nil {
		log.Fatalf("Ошибка создания бота: %v", err)
	}

	// Создание и настройка сервиса расписания
	schedulerService := services.NewSchedulerService(botService.Bot, menuService)
	botService.SchedulerService = schedulerService

	// Запуск бота
	log.Println("Запуск бота...")
	botService.Start()
}
