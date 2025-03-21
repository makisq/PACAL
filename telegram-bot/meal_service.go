package main

import (
	"accses"
	"math/rand"
	"time"
)

func GenerateBreakfast() []string {
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

// Аналогично для GenerateLunch и GenerateDinner
