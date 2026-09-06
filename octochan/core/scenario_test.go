package core

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

func TestParseScenarioData(t *testing.T) {
	t.Run("YAML со списком targets", func(t *testing.T) {
		data := []byte(`
service: psql_tuning_params_se
parameters:
  role: standalone
targets:
  - svm_ci: CI-1
  - svm_ci: ["CI-2", "CI-3"]
`)
		s, err := ParseScenarioData(data)
		if err != nil {
			t.Fatalf("ParseScenarioData: %v", err)
		}
		if s.Service != "psql_tuning_params_se" {
			t.Errorf("service = %q", s.Service)
		}
		if len(s.Targets) != 2 {
			t.Fatalf("получено %d targets, ожидалось 2", len(s.Targets))
		}
		if got := s.Targets[0].GetCIs(); len(got) != 1 || got[0] != "CI-1" {
			t.Errorf("target[0].GetCIs() = %v", got)
		}
		if got := s.Targets[1].GetCIs(); len(got) != 2 {
			t.Errorf("target[1].GetCIs() = %v, ожидалось 2 элемента", got)
		}
	})

	t.Run("JSON с одиночным targets", func(t *testing.T) {
		data := []byte(`{"service":"s","targets":{"svm_ci":"CI-9","ip":"10.0.0.1"}}`)
		s, err := ParseScenarioData(data)
		if err != nil {
			t.Fatalf("ParseScenarioData: %v", err)
		}
		if len(s.Targets) != 1 || s.Targets[0].SVMCI != "CI-9" || s.Targets[0].IP != "10.0.0.1" {
			t.Errorf("targets разобраны неверно: %+v", s.Targets)
		}
	})

	t.Run("без service", func(t *testing.T) {
		if _, err := ParseScenarioData([]byte(`targets: [{svm_ci: CI-1}]`)); err == nil {
			t.Error("ожидалась ошибка об отсутствующем service")
		}
	})

	t.Run("мусор", func(t *testing.T) {
		if _, err := ParseScenarioData([]byte("\t\x00не конфиг")); err == nil {
			t.Error("ожидалась ошибка разбора")
		}
	})
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
