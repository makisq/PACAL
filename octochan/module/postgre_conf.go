package module

import (
	"fmt"
	"octochan/core"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type PostgresConfigFilesModule struct {
	data *core.ScenarioData
}

func NewPostgresConfigFilesModule(data *core.ScenarioData) (core.ScenarioModule, error) {
	return &PostgresConfigFilesModule{data: data}, nil
}

func (m *PostgresConfigFilesModule) Validate() error {
	requiredParams := []string{
		"port",
		"config_list",
		"itemname",
	}

	for _, param := range requiredParams {
		if _, ok := m.data.Parameters[param]; !ok {
			return fmt.Errorf("отсутствует обязательный параметр: %s", param)
		}
	}

	if len(m.data.Targets) == 0 {
		return fmt.Errorf("не указаны целевые серверы (targets)")
	}

	for i, target := range m.data.Targets {
		if target.SVMCI == "" {
			return fmt.Errorf("пустой SVMCI в target #%d", i+1)
		}
	}

	return nil
}

func (m *PostgresConfigFilesModule) GenerateRequests() ([]*core.APIRequest, error) {
	var requests []*core.APIRequest
	apiURL := viper.GetString("defaults.api_url")
	token := viper.GetString("defaults.api_token")

	// Базовый payload
	basePayload := map[string]interface{}{
		"service":  "postgresql_se_get_config_files",
		"start_at": "now",
		"datetime": time.Now().Format(time.RFC3339),
		"params": map[string]interface{}{
			"hosts": []map[string]interface{}{},
		},
	}

	hostParams := map[string]interface{}{
		"port":        m.data.Parameters["port"],
		"itemname":    m.data.Parameters["itemname"],
		"config_list": strings.Split(m.data.Parameters["config_list"].(string), ","),
	}

	basePayload["params"].(map[string]interface{})["hosts"] = append(
		basePayload["params"].(map[string]interface{})["hosts"].([]map[string]interface{}),
		hostParams,
	)


	for _, target := range m.data.Targets {
		payload := core.DeepCopyMap(basePayload)

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


