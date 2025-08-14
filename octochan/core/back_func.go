package core

import (
	"fmt"
	"os"
)

func Any[T any](slice []T, predicate func(T) bool) bool {
	for _, v := range slice {
		if predicate(v) {
			return true
		}
	}
	return false
}

func equalValues(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func getValueOrNA(value interface{}) interface{} {
	if value == nil {
		return "N/A"
	}
	return value
}

func exitWithError(err error) {
	fmt.Printf("Ошибка: %v\n", err)
	os.Exit(1)
}
