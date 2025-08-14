package module

import (
	"fmt"
	"octochan/core"
	"time"

	"github.com/spf13/viper"
)

type PgBouncerTuningModule struct {
	data *core.ScenarioData
}

func NewPgBouncerTuningModule(data *core.ScenarioData) (core.ScenarioModule, error) {
	return &PgBouncerTuningModule{data: data}, nil
}

func (m *PgBouncerTuningModule) Validate() error {
	if len(m.data.Targets) == 0 {
		return fmt.Errorf("не указаны целевые серверы (targets)")
	}

	for i, target := range m.data.Targets {
		if target.SVMCI == "" {
			return fmt.Errorf("пустой SVMCI в target #%d", i+1)
		}
	}

	requiredParams := map[string]string{
		"role":    "роль сервера (standalone/replica)",
		"version": "версия ПО",
	}

	for param, desc := range requiredParams {
		if _, ok := m.data.Parameters[param]; !ok {
			return fmt.Errorf("отсутствует обязательный параметр: %s (%s)", param, desc)
		} else if val, ok := m.data.Parameters[param].(string); ok && val == "" {
			return fmt.Errorf("параметр %s не может быть пустым", param)
		}
	}

	return nil
}

func (m *PgBouncerTuningModule) GenerateRequests() ([]*core.APIRequest, error) {
	var requests []*core.APIRequest
	apiURL := viper.GetString("defaults.api_url")
	token := viper.GetString("defaults.api_token")

	basePayload := map[string]interface{}{
		"service":  "psqlse_tuningpgbouncer",
		"start_at": "now",
		"datetime": time.Now().Format(time.RFC3339),
		"params":   make(map[string]interface{}),
	}

	for k, v := range m.data.Parameters {
		if k != "role" && k != "version" {
			basePayload["params"].(map[string]interface{})[k] = v
		}
	}

	for _, target := range m.data.Targets {
		payload := core.DeepCopyMap(basePayload)
		params := payload["params"].(map[string]interface{})

		params["hosts"] = []map[string]interface{}{
			{
				"svm_ci":  target.SVMCI,
				"role":    m.data.Parameters["role"],
				"version": m.data.Parameters["version"],
			},
		}

		items, err := core.PrepareScenarioItems([]core.Target{target})
		if err != nil {
			return nil, err
		}
		payload["items"] = items

		requests = append(requests, &core.APIRequest{
			Method: "POST",
			URL:    apiURL,
			Headers: map[string]string{
				"accept":        "application/json",
				"Authorization": "Token " + token,
				"Content-Type":  "application/json",
			},
			Body: payload,
		})
	}

	return requests, nil
}
