package main

import (
	"sync"
	"time"
)

type UserData struct {
	ChatID        int64
	DesiredWeight float64
	CurrentWeight float64
	Progress      []WeightRecord
	MealTimes     map[string]string
}

type WeightRecord struct {
	Date   time.Time
	Weight float64
}

var (
	userDataMap   = make(map[int64]*UserData)
	userDataMutex sync.Mutex
)

func SetWeight(chatID int64, weight float64) {
	userDataMutex.Lock()
	defer userDataMutex.Unlock()

	userData := getUserData(chatID)
	userData.CurrentWeight = weight
	userData.Progress = append(userData.Progress, WeightRecord{Date: time.Now(), Weight: weight})
}

func SetDesiredWeight(chatID int64, weight float64) {
	userDataMutex.Lock()
	defer userDataMutex.Unlock()

	userData := getUserData(chatID)
	userData.DesiredWeight = weight
}

func GetProgress(chatID int64) []WeightRecord {
	userDataMutex.Lock()
	defer userDataMutex.Unlock()

	userData := getUserData(chatID)
	return userData.Progress
}

func SetMealTime(chatID int64, mealType string, timeStr string) {
	userDataMutex.Lock()
	defer userDataMutex.Unlock()

	userData := getUserData(chatID)
	userData.MealTimes[mealType] = timeStr
}

func getUserData(chatID int64) *UserData {
	if userData, ok := userDataMap[chatID]; ok {
		return userData
	}

	userData := &UserData{
		ChatID:    chatID,
		MealTimes: make(map[string]string),
	}
	userDataMap[chatID] = userData
	return userData
}
