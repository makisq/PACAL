package rlm

import (
	"net/http"
	"testing"
)

func TestExtractTaskID(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    string
		wantErr bool
	}{
		{"строка", map[string]any{"id": "abc"}, "abc", false},
		{"целое из JSON", map[string]any{"id": float64(42)}, "42", false},
		{"int", map[string]any{"id": 42}, "42", false},
		{"нет поля", map[string]any{}, "", true},
		{"null", map[string]any{"id": nil}, "", true},
		{"неподдерживаемый тип", map[string]any{"id": []any{1}}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractTaskID(tt.payload)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ошибка = %v, ожидалась = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("получено %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestDeepCopyMapCopiesSlices(t *testing.T) {
	// Прежняя версия копировала только вложенные map, а срезы оставались
	// общими между всеми запросами.
	original := map[string]any{
		"hosts": []any{map[string]any{"svm_ci": "CI-1"}},
	}

	clone := DeepCopyMap(original)
	clone["hosts"].([]any)[0].(map[string]any)["svm_ci"] = "CI-CHANGED"

	if got := original["hosts"].([]any)[0].(map[string]any)["svm_ci"]; got != "CI-1" {
		t.Errorf("оригинал изменился: %q", got)
	}
}

func TestTaskStateOf(t *testing.T) {
	cases := map[string]TaskState{
		"completed":   TaskStateSucceeded,
		"SUCCESS":     TaskStateSucceeded,
		"failed":      TaskStateFailed,
		"cancelled":   TaskStateFailed,
		"in_progress": TaskStateRunning,
		"неизвестно":  TaskStateUnknown,
	}

	for status, want := range cases {
		if got := TaskStateOf(status); got != want {
			t.Errorf("TaskStateOf(%q) = %v, ожидалось %v", status, got, want)
		}
	}
}

func TestAPIErrorHelpers(t *testing.T) {
	notFound := &APIError{StatusCode: http.StatusNotFound}
	if !IsNotFound(notFound) {
		t.Error("IsNotFound должен распознавать 404")
	}

	// Раньше принадлежность ошибки к table_id определялась поиском подстроки
	// в тексте любой ошибки, включая сетевую.
	tableErr := &APIError{StatusCode: 400, Body: "Указан неверный идентификатор таблицы"}
	if !IsTableIDError(tableErr) {
		t.Error("IsTableIDError должен распознавать отказ по таблице")
	}
	if IsTableIDError(notFound) {
		t.Error("404 не является ошибкой table_id")
	}
}
