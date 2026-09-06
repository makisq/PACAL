package module

import (
	"context"
	"fmt"
	"time"

	"octochan/core"
)

// defaultTableID — таблица инвентаря по умолчанию.
// Раньше это значение было зашито внутри core.PrepareScenarioItems.
const defaultTableID = "psqlseclusterstandalone"

// PgBouncerTuningModule настраивает pgbouncer на целевых серверах.
type PgBouncerTuningModule struct {
	data   *core.ScenarioData
	params Params
}

// NewPgBouncerTuningModule создаёт модуль сервиса psqlse_tuningpgbouncer.
func NewPgBouncerTuningModule(data *core.ScenarioData) (core.ScenarioModule, error) {
	return &PgBouncerTuningModule{data: data, params: Params(data.Parameters)}, nil
}

func (m *PgBouncerTuningModule) Validate() error {
	if len(m.data.Targets) == 0 {
		return fmt.Errorf("не указаны целевые серверы (targets)")
	}
	for i, target := range m.data.Targets {
		if target.SVMCI == "" {
			return fmt.Errorf("пустой svm_ci в target #%d", i+1)
		}
	}
	return m.params.RequireNonEmpty("role", "version")
}

func (m *PgBouncerTuningModule) GenerateRequests(_ context.Context, _ *core.APIClient) ([]*core.APIRequest, error) {
	// Параметры настройки — всё, кроме служебных role/version,
	// которые уезжают в описание хоста.
	tuning := make(map[string]any, len(m.data.Parameters))
	for k, v := range m.data.Parameters {
		if k != "role" && k != "version" {
			tuning[k] = v
		}
	}

	tableID := m.params.String("table_id", defaultTableID)
	requests := make([]*core.APIRequest, 0, len(m.data.Targets))

	for _, target := range m.data.Targets {
		params := core.DeepCopyMap(tuning)
		params["hosts"] = []any{
			map[string]any{
				"svm_ci":  target.SVMCI,
				"role":    m.params.String("role", ""),
				"version": m.params.String("version", ""),
			},
		}

		items, err := core.PrepareScenarioItems([]core.Target{target}, tableID)
		if err != nil {
			return nil, err
		}

		requests = append(requests, &core.APIRequest{
			Method: "POST",
			Body: map[string]any{
				"service":  "psqlse_tuningpgbouncer",
				"start_at": "now",
				"datetime": time.Now().Format(time.RFC3339),
				"params":   params,
				"items":    items,
			},
		})
	}

	return requests, nil
}
