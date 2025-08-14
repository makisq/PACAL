package module

import (
	"fmt"
	"octochan/core"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type PsqlTuningParamsModule struct {
	data *core.ScenarioData
}

func NewPsqlTuningParamsModule(data *core.ScenarioData) (core.ScenarioModule, error) {
	return &PsqlTuningParamsModule{data: data}, nil
}

func (m *PsqlTuningParamsModule) Validate() error {
	if len(m.data.Targets) == 0 {
		return fmt.Errorf("не указаны целевые серверы (targets)")
	}

	for i, target := range m.data.Targets {
		if target.SVMCI == "" {
			return fmt.Errorf("пустой SVMCI в target #%d", i+1)
		}
	}

	requiredParams := map[string]string{
		"role":       "роль сервера (master/replica)",
		"port":       "порт подключения",
		"ip_replics": "IP адреса реплик",
		"svm_ip":     "IP адреса SVM",
		"svm_ci":     "CI номер SVM",
	}

	for param, desc := range requiredParams {
		if _, ok := m.data.Parameters[param]; !ok {
			return fmt.Errorf("отсутствует обязательный параметр: %s (%s)", param, desc)
		} else if val, ok := m.data.Parameters[param].(string); ok && val == "" {
			return fmt.Errorf("параметр %s не может быть пустым", param)
		}
	}

	if ipReplics, ok := m.data.Parameters["ip_replics"].(string); ok {
		ips := strings.Split(ipReplics, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				return fmt.Errorf("обнаружен пустой IP в списке реплик")
			}
		}
	}

	return nil
}

func (m *PsqlTuningParamsModule) GenerateRequests() ([]*core.APIRequest, error) {
	var requests []*core.APIRequest
	apiURL := viper.GetString("defaults.api_url")
	token := viper.GetString("defaults.api_token")

	basePayload := map[string]interface{}{
		"service":  "psql_tuning_params_se",
		"start_at": "now",
		"datetime": time.Now().Format(time.RFC3339),
		"params": map[string]interface{}{
			"parameters": []interface{}{},
		},
	}

	for k, v := range m.data.Parameters {
		switch k {
		case "restart":
			basePayload["params"].(map[string]interface{})["restart"] = v
		case "port":
			basePayload["params"].(map[string]interface{})["port"] = v
		case "role":
			basePayload["params"].(map[string]interface{})["role"] = v
		case "ip_replics":
			basePayload["params"].(map[string]interface{})["ip_replics"] = v
		case "svm_ci":
			basePayload["params"].(map[string]interface{})["svm_ci"] = v
		case "svm_ip":
			basePayload["params"].(map[string]interface{})["svm_ip"] = v
		case "skip_sm_conflicts":
			basePayload["params"].(map[string]interface{})["skip_sm_conflicts"] = v
		default:
			params := basePayload["params"].(map[string]interface{})["parameters"].([]interface{})
			basePayload["params"].(map[string]interface{})["parameters"] = append(params, map[string]interface{}{
				"name":  k,
				"value": v,
			})
		}
	}

	for _, target := range m.data.Targets {
		payload := core.DeepCopyMap(basePayload)

		params := payload["params"].(map[string]interface{})
		params["svm_ci"] = target.SVMCI

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
