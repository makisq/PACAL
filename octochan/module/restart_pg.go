package module

import (
	"encoding/json"
	"fmt"
	"log"
	"octochan/core"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type PangolinRestartItem struct {
	TableID               string `json:"table_id"`
	InvsvmCISvm           string `json:"invsvm_ci_svm,omitempty"`
	InvsvmIP              string `json:"invsvm_ip,omitempty"`
	InvsvmAliaces         string `json:"invsvm_aliaces,omitempty"`
	InvinstCIInst         string `json:"invinst_ci_inst,omitempty"`
	InvinstInstName       string `json:"invinst_inst_name,omitempty"`
	InvirCIIr             string `json:"invir_ci_ir,omitempty"`
	InvirIrName           string `json:"invir_ir_name,omitempty"`
	InvstendEnvironment   string `json:"invstend_environment,omitempty"`
	InvstendCIStend       string `json:"invstend_ci_stend,omitempty"`
	InvstendStendName     string `json:"invstend_stend_name,omitempty"`
	InvclusterCICluster   string `json:"invcluster_ci_cluster,omitempty"`
	InvclusterClusterName string `json:"invcluster_cluster_name,omitempty"`
	InvirpgseCIIr         string `json:"invirpgse_ci_ir,omitempty"`
	InvirpgseIrName       string `json:"invirpgse_ir_name,omitempty"`
}

type PangolinRestartParams struct {
	Restart         string `json:"restart"`
	SkipSMConflicts bool   `json:"skip_sm_conflicts"`
	Confirm         bool   `json:"confirm"`
}

type PangolinRestartPayload struct {
	Service  string                `json:"service"`
	StartAt  string                `json:"start_at"`
	Datetime string                `json:"datetime"`
	Params   PangolinRestartParams `json:"params"`
	Items    []PangolinRestartItem `json:"items"`
}

type PangolinRestartModule struct {
	data *core.ScenarioData
}

func NewPangolinRestartModule(data *core.ScenarioData) (core.ScenarioModule, error) {
	return &PangolinRestartModule{
		data: data,
	}, nil
}

func (m *PangolinRestartModule) Validate() error {
	if len(m.data.Items) == 0 && len(m.data.Targets) == 0 {
		return fmt.Errorf("не указаны items или targets для перезапуска")
	}

	// Проверяем items, если они есть
	if len(m.data.Items) > 0 {
		for i, item := range m.data.Items {
			hasRequiredField := false

			if m.getStringFromMap(item, "invsvm_ci_svm") != "" {
				hasRequiredField = true
			}
			if m.getStringFromMap(item, "invsvm_ip") != "" {
				hasRequiredField = true
			}
			if m.getStringFromMap(item, "invsvm_aliaces") != "" {
				hasRequiredField = true
			}
			if m.getStringFromMap(item, "invinst_ci_inst") != "" {
				hasRequiredField = true
			}
			if m.getStringFromMap(item, "invir_ci_ir") != "" {
				hasRequiredField = true
			}

			if !hasRequiredField {
				return fmt.Errorf("item #%d должен содержать хотя бы один из параметров: invsvm_ci_svm, invsvm_ip, invsvm_aliaces, invinst_ci_inst, invir_ci_ir", i+1)
			}
		}
	}

	// Проверяем targets, если они есть
	if len(m.data.Targets) > 0 {
		for i, target := range m.data.Targets {
			if target.SVMCI == "" && target.IP == "" {
				return fmt.Errorf("target #%d должен содержать хотя бы один из параметров: svm_ci или IP", i+1)
			}
		}
	}

	// Проверяем обязательные параметры
	requiredParams := []string{"restart", "skip_sm_conflicts", "confirm"}
	for _, param := range requiredParams {
		if _, ok := m.data.Parameters[param]; !ok {
			return fmt.Errorf("отсутствует обязательный параметр: %s", param)
		}
	}

	return nil
}

func (m *PangolinRestartModule) GenerateRequests() ([]*core.APIRequest, error) {
	var requests []*core.APIRequest

	// Создаем запросы для items
	if len(m.data.Items) > 0 {
		req, err := m.createRequestFromItems()
		if err != nil {
			return nil, fmt.Errorf("ошибка создания запроса из items: %w", err)
		}
		requests = append(requests, req)
	}

	// Создаем запросы для targets (асинхронно по одному запросу на каждый target)
	if len(m.data.Targets) > 0 {
		for _, target := range m.data.Targets {
			req, err := m.createRequestFromTarget(target)
			if err != nil {
				return nil, fmt.Errorf("ошибка создания запроса из target: %w", err)
			}
			requests = append(requests, req)
		}
	}

	log.Printf("✅ Создано %d запросов для перезапуска pangolin", len(requests))
	return requests, nil
}

func (m *PangolinRestartModule) createRequestFromItems() (*core.APIRequest, error) {
	url := viper.GetString("defaults.api_url")
	token := viper.GetString("defaults.api_token")

	// Подготавливаем параметры
	params := PangolinRestartParams{
		Restart:         m.getStringParam("restart", ""),
		SkipSMConflicts: m.getBoolParam("skip_sm_conflicts", false),
		Confirm:         m.getBoolParam("confirm", false),
	}

	// Подготавливаем items
	var items []PangolinRestartItem
	for _, itemData := range m.data.Items {
		tableID := m.getStringFromMap(itemData, "table_id")
		if tableID == "" {
			tableID = "pangolinunique" // значение по умолчанию
		}

		item := PangolinRestartItem{
			TableID:               tableID,
			InvsvmCISvm:           m.getStringFromMap(itemData, "invsvm_ci_svm"),
			InvsvmIP:              m.getStringFromMap(itemData, "invsvm_ip"),
			InvsvmAliaces:         m.getStringFromMap(itemData, "invsvm_aliaces"),
			InvinstCIInst:         m.getStringFromMap(itemData, "invinst_ci_inst"),
			InvinstInstName:       m.getStringFromMap(itemData, "invinst_inst_name"),
			InvirCIIr:             m.getStringFromMap(itemData, "invir_ci_ir"),
			InvirIrName:           m.getStringFromMap(itemData, "invir_ir_name"),
			InvstendEnvironment:   m.getStringFromMap(itemData, "invstend_environment"),
			InvstendCIStend:       m.getStringFromMap(itemData, "invstend_ci_stend"),
			InvstendStendName:     m.getStringFromMap(itemData, "invstend_stend_name"),
			InvclusterCICluster:   m.getStringFromMap(itemData, "invcluster_ci_cluster"),
			InvclusterClusterName: m.getStringFromMap(itemData, "invcluster_cluster_name"),
			InvirpgseCIIr:         m.getStringFromMap(itemData, "invirpgse_ci_ir"),
			InvirpgseIrName:       m.getStringFromMap(itemData, "invirpgse_ir_name"),
		}
		items = append(items, item)
	}

	return m.createAPIRequest(url, token, params, items)
}

func (m *PangolinRestartModule) createRequestFromTarget(target core.Target) (*core.APIRequest, error) {
	url := viper.GetString("defaults.api_url")
	token := viper.GetString("defaults.api_token")
	params := PangolinRestartParams{
		Restart:         m.getStringParam("restart", ""),
		SkipSMConflicts: m.getBoolParam("skip_sm_conflicts", false),
		Confirm:         m.getBoolParam("confirm", false),
	}

	item := PangolinRestartItem{
		TableID:     "pangolinunique",
		InvsvmCISvm: target.SVMCI,
		InvsvmIP:    target.IP,
	}
	var items []PangolinRestartItem
	if len(target.CIList) > 0 {
		for _, ci := range target.CIList {
			items = append(items, PangolinRestartItem{
				TableID:     "pangolinunique",
				InvsvmCISvm: ci,
			})
		}
	} else {
		items = append(items, item)
	}

	return m.createAPIRequest(url, token, params, items)
}

func (m *PangolinRestartModule) createAPIRequest(url, token string, params PangolinRestartParams, items []PangolinRestartItem) (*core.APIRequest, error) {

	payload := PangolinRestartPayload{
		Service:  "pangolin_restart",
		StartAt:  "now",
		Datetime: time.Now().Format("2006-01-02T15:04:05-07:00"),
		Params:   params,
		Items:    items,
	}

	jsonData, _ := json.MarshalIndent(payload, "", "  ")
	log.Printf("Сформированный запрос для pangolin_restart:\n%s", string(jsonData))

	var bodyMap map[string]interface{}
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}
	if err := json.Unmarshal(jsonBytes, &bodyMap); err != nil {
		return nil, fmt.Errorf("ошибка преобразования в map: %w", err)
	}

	return &core.APIRequest{
		Method: "POST",
		URL:    url,
		Headers: map[string]string{
			"Authorization": "Token " + token,
			"Content-Type":  "application/json",
			"Accept":        "application/json",
		},
		Body: bodyMap,
	}, nil
}

func (m *PangolinRestartModule) getStringParam(key string, defaultValue string) string {
	if val, ok := m.data.Parameters[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func (m *PangolinRestartModule) getBoolParam(key string, defaultValue bool) bool {
	if val, ok := m.data.Parameters[key]; ok {
		switch v := val.(type) {
		case bool:
			return v
		case string:
			return strings.ToLower(v) == "true" || v == "1"
		case int:
			return v != 0
		}
	}
	return defaultValue
}

func (m *PangolinRestartModule) getStringFromMap(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
		if arr, ok := val.([]interface{}); ok && len(arr) > 0 {
			if str, ok := arr[0].(string); ok {
				return str
			}
		}
	}
	return ""
}
