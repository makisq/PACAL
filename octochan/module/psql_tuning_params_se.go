package module

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"octochan/core"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

type IntelItem struct {
	TableID     string `json:"table_id"`
	InvsvmCISvm string `json:"invsvm_ci_svm"`
	TableRowID  string `json:"table_row_id,omitempty"`
}

type IntelPayload struct {
	Service string      `json:"service"`
	StartAt string      `json:"start_at"`
	Items   []IntelItem `json:"items"`
}

type DBParam struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
	Unit  string      `json:"unit,omitempty"`
}

type MainParams struct {
	IpReplics       string    `json:"ip_replics"`
	DBParams        []DBParam `json:"db_params"`
	Port            int       `json:"port"`
	Restart         bool      `json:"restart"`
	Hugepages       bool      `json:"hugepages"`
	Role            string    `json:"role"`
	SkipSMConflicts bool      `json:"skip_sm_conflicts"`
	SvmCI           string    `json:"svm_ci"`
	SvmIP           string    `json:"svm_ip"`
	TaskID          int       `json:"task_id"`
}

type MainItem struct {
	TableID     string `json:"table_id"`
	InvsvmCISvm string `json:"invsvm_ci_svm"`
}

type MainPayload struct {
	Datetime string     `json:"datetime"`
	Items    []MainItem `json:"items"`
	Params   MainParams `json:"params"`
	Service  string     `json:"service"`
	StartAt  string     `json:"start_at"`
}

type PsqlTuningParamsModule struct {
	data          *core.ScenarioData
	intelTaskMap  map[string]int
	useTableRowID bool
	lastStatuses  map[int]string
}

type IntelTaskResult struct {
	TaskID      int    `json:"id"`
	Status      string `json:"status"`
	TableID     string `json:"table_id"`
	CompletedAt string `json:"completed_at"`
}

func NewPsqlTuningParamsModule(data *core.ScenarioData) (core.ScenarioModule, error) {
	return &PsqlTuningParamsModule{
		data:          data,
		intelTaskMap:  make(map[string]int),
		useTableRowID: false,
		lastStatuses:  make(map[int]string),
	}, nil
}

func (m *PsqlTuningParamsModule) Validate() error {
	if len(m.data.Targets) == 0 {
		return fmt.Errorf("не указаны целевые серверы (targets)")
	}

	for i, target := range m.data.Targets {
		if len(target.GetCIs()) == 0 { // ← ИСПРАВЛЕННАЯ ПРОВЕРКА
			return fmt.Errorf("пустой SVMCI в target #%d", i+1)
		}
	}

	requiredParams := []string{"role", "port", "svm_ip", "hugepages"}
	for _, param := range requiredParams {
		if val, ok := m.data.Parameters[param]; !ok || val == "" {
			return fmt.Errorf("отсутствует обязательный параметр: %s", param)
		}
	}

	if tuningParams, ok := m.data.Parameters["parameters"]; ok {
		if paramList, ok := tuningParams.([]interface{}); ok {
			for i, p := range paramList {
				if param, ok := p.(map[string]interface{}); ok {
					if param["name"] == "" || param["setting"] == "" {
						return fmt.Errorf("неполный параметр настройки #%d: требуется name и setting", i+1)
					}
				} else {
					return fmt.Errorf("неверный формат параметра настройки #%d", i+1)
				}
			}
		}
	}

	return nil
}

func (m *PsqlTuningParamsModule) GenerateRequests() ([]*core.APIRequest, error) {
	var requests []*core.APIRequest

	intelTaskMap, intelTasks, err := m.startIntelForAllTargets()
	if err != nil {
		return nil, fmt.Errorf("ошибка запуска разведки: %w", err)
	}

	log.Printf("✅ Успешно запущено %d задач разведки: %v", len(intelTasks), intelTasks)

	if len(intelTasks) == 0 {
		return nil, fmt.Errorf("не удалось запустить задачи разведки")
	}

	log.Printf("⏳ Ожидаем завершения %d задач разведки...", len(intelTasks))
	results, err := m.MonitorIntelTasks(intelTasks)
	if err != nil {
		return nil, fmt.Errorf("ошибка мониторинга задач разведки: %w", err)
	}

	log.Printf("✅ Все задачи разведки завершены успешно")

	for _, target := range m.data.Targets {
		for _, svmCI := range target.GetCIs() {
			taskID, exists := intelTaskMap[svmCI]
			if !exists {
				return nil, fmt.Errorf("не найден taskID для CI %s", svmCI)
			}

			intelResult, exists := results[taskID]
			if !exists {
				return nil, fmt.Errorf("не найден результат разведки для taskID %d", taskID)
			}

			req, err := m.tryCreateMainRequest(svmCI, taskID, intelResult.TableID)
			if err != nil {
				return nil, fmt.Errorf("ошибка создания основного запроса для CI %s: %w", svmCI, err)
			}
			requests = append(requests, req)
		}
	}
	for i, req := range requests {
		jsonData, _ := json.MarshalIndent(req.Body, "", "  ")
		log.Printf("🔍 Запрос #%d:\n%s", i+1, string(jsonData))
	}

	log.Printf("✅ Создано %d основных запросов", len(requests))
	return requests, nil
}

func (m *PsqlTuningParamsModule) tryCreateMainRequest(svmCI string, taskID int, intelTableID string) (*core.APIRequest, error) {

	tableIDs := []string{
		"pangolinunique",
		"psqlsecluster",
		"psqlseclusterstandalone",
		"postgresql_instances",
	}

	var lastErr error
	for _, tableID := range tableIDs {
		req, err := m.createMainRequest(svmCI, taskID, tableID)
		if err == nil {
			return req, nil
		}

		lastErr = err
		log.Printf("⚠️  Ошибка с table_id '%s' для основного запроса: %v", tableID, err)

		if strings.Contains(err.Error(), "table_id") || strings.Contains(err.Error(), "Указан неверный идентификатор таблицы") {
			continue
		}

		return nil, err
	}

	return nil, fmt.Errorf("не удалось найти working table_id для основного запроса CI %s: %w", svmCI, lastErr)
}

func (m *PsqlTuningParamsModule) startIntelForAllTargets() (map[string]int, []int, error) {
	intelTaskMap := make(map[string]int)
	var intelTasks []int

	totalCIs := 0
	for _, target := range m.data.Targets {
		totalCIs += len(target.GetCIs())
	}

	if totalCIs == 0 {
		return nil, nil, fmt.Errorf("нет CI для запуска разведки")
	}

	log.Printf("🔄 Запуск разведки для %d CI", totalCIs)

	errorChan := make(chan error, totalCIs)
	resultChan := make(chan struct {
		svmCI  string
		taskID int
	}, totalCIs)

	var wg sync.WaitGroup
	for _, target := range m.data.Targets {
		for _, svmCI := range target.GetCIs() {
			wg.Add(1)
			go func(ci string) {
				defer wg.Done()

				taskID, err := m.sendIntelRequest(ci)
				if err != nil {
					errorChan <- fmt.Errorf("ошибка разведки для CI %s: %w", ci, err)
					return
				}

				resultChan <- struct {
					svmCI  string
					taskID int
				}{svmCI: ci, taskID: taskID}
			}(svmCI)
		}
	}

	wg.Wait()

	select {
	case err := <-errorChan:

		close(errorChan)
		close(resultChan)
		return nil, nil, err
	default:
	}

	close(errorChan)
	close(resultChan)

	for result := range resultChan {
		intelTaskMap[result.svmCI] = result.taskID
		intelTasks = append(intelTasks, result.taskID)
	}

	if len(intelTasks) == 0 {
		return nil, nil, fmt.Errorf("не удалось запустить задачи разведки: нет результатов")
	}

	return intelTaskMap, intelTasks, nil
}

func (m *PsqlTuningParamsModule) sendIntelRequest(svmCI string) (int, error) {
	tableIDs := []string{
		"pangolinunique",
		"psqlsecluster",
		"psqlseclusterstandalone",
		"postgresql_instances",
	}

	for _, tableID := range tableIDs {
		taskID, err := m.trySendIntelRequest(svmCI, tableID, false)
		if err == nil {
			return taskID, nil
		}

		if strings.Contains(err.Error(), "table_id") {
			log.Printf("⚠️  Обнаружена ошибка table_id '%s', пробуем следующий вариант", tableID)
			continue
		}

		if strings.Contains(err.Error(), "table_row_id") {
			log.Printf("⚠️  Обнаружена ошибка table_row_id, пробуем с table_row_id: \"None\"")
			taskID, err = m.trySendIntelRequest(svmCI, tableID, true)
			if err == nil {
				return taskID, nil
			}
		}

		return 0, err
	}

	return 0, fmt.Errorf("не удалось найти working table_id для CI %s", svmCI)
}

func (m *PsqlTuningParamsModule) trySendIntelRequest(svmCI, tableID string, useTableRowID bool) (int, error) {
	apiURL := viper.GetString("defaults.api_url")
	token := viper.GetString("defaults.api_token")

	intelPayload := map[string]interface{}{
		"service":  "psql_tuning_params_se_sys",
		"start_at": "now",
		"items": []map[string]interface{}{
			{
				"table_id":      tableID,
				"invsvm_ci_svm": svmCI,
			},
		},
	}

	if useTableRowID {
		intelPayload["items"].([]map[string]interface{})[0]["table_row_id"] = "None"
		m.useTableRowID = true
	}

	jsonData, err := json.Marshal(intelPayload)
	if err != nil {
		return 0, fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("ошибка HTTP запроса: %w", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("ошибка API (код %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	log.Printf("✅ Задача разведки создана для CI %s с table_id %s: %极", svmCI, tableID, result.ID)
	return result.ID, nil
}

func (m *PsqlTuningParamsModule) createMainRequest(svmCI string, taskID int, tableID string) (*core.APIRequest, error) {
	url := viper.GetString("defaults.api_url")
	token := viper.GetString("defaults.api_token")

	ipReplicsStr, ok := m.data.Parameters["ip_replics"].(string)
	if !ok {
		return nil, fmt.Errorf("ip_replics должен быть строкой")
	}

	ipReplics := strings.Split(ipReplicsStr, ",")
	var targetIP string
	for _, target := range m.data.Targets {
		for j, ci := range target.GetCIs() {
			if ci == svmCI && j < len(ipReplics) {
				targetIP = strings.TrimSpace(ipReplics[j])
				break
			}
		}
		if targetIP != "" {
			break
		}
	}

	if targetIP == "" {
		if len(ipReplics) > 0 {
			targetIP = strings.TrimSpace(ipReplics[0])
		} else {
			return nil, fmt.Errorf("не найден IP для CI %s", svmCI)
		}
	}

	params := MainParams{
		IpReplics:       targetIP,
		DBParams:        m.prepareDBParams(),
		Port:            m.getIntParam("port", 5432),
		Restart:         m.getBoolParam("restart", false),
		Hugepages:       m.getBoolParam("hugepages", false),
		Role:            m.getStringParam("role", ""),
		SkipSMConflicts: m.getBoolParam("skip_sm_conflicts", true),
		SvmCI:           svmCI,
		SvmIP:           m.getStringParam("svm_ip", ""),
		TaskID:          taskID,
	}

	item := MainItem{
		TableID:     tableID,
		InvsvmCISvm: svmCI,
	}

	mainPayload := MainPayload{
		Datetime: time.Now().Format("2006-01-02T15:04:05-07:00"),
		Items:    []MainItem{item},
		Params:   params,
		Service:  "psql_tuning_params_se",
		StartAt:  "now",
	}

	jsonData, _ := json.MarshalIndent(mainPayload, "", "  ")
	log.Printf("Сформированный запрос для CI %s:\n%s", svmCI, string(jsonData))

	var bodyMap map[string]interface{}
	jsonBytes, err := json.Marshal(mainPayload)
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

func (m *PsqlTuningParamsModule) prepareDBParams() []DBParam {
	var dbParams []DBParam

	if tuningParams, ok := m.data.Parameters["parameters"]; ok {
		if paramList, ok := tuningParams.([]interface{}); ok {
			for _, p := range paramList {
				if param, ok := p.(map[string]interface{}); ok {
					var value interface{} = getString(param["setting"])
					if num, err := strconv.Atoi(getString(param["setting"])); err == nil {
						value = num
					}

					dbParam := DBParam{
						Name:  getString(param["name"]),
						Value: value,
						Unit:  getString(param["unit"]),
					}
					dbParams = append(dbParams, dbParam)
				}
			}
		}
	}

	return dbParams
}

func getString(value interface{}) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", value)
}

func (m *PsqlTuningParamsModule) getStringParam(key string, defaultValue string) string {
	if val, ok := m.data.Parameters[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func (m *PsqlTuningParamsModule) getBoolParam(key string, defaultValue bool) bool {
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

func (m *PsqlTuningParamsModule) getIntParam(key string, defaultValue int) int {
	if val, ok := m.data.Parameters[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case string:
			if num, err := strconv.Atoi(v); err == nil {
				return num
			}
		}
	}
	return defaultValue
}

func (m *PsqlTuningParamsModule) MonitorIntelTasks(taskIDs []int) (map[int]*IntelTaskResult, error) {
	results := make(map[int]*IntelTaskResult)
	var wg sync.WaitGroup
	var mu sync.Mutex
	errorChan := make(chan error, len(taskIDs))

	for _, taskID := range taskIDs {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			result, err := m.waitForIntelTaskCompletion(id)
			if err != nil {
				errorChan <- fmt.Errorf("ошибка ожидания задачи %极: %w", id, err)
				return
			}

			mu.Lock()
			results[id] = result
			mu.Unlock()
		}(taskID)
	}

	wg.Wait()
	close(errorChan)

	for err := range errorChan {
		return nil, err
	}

	return results, nil
}

func (m *PsqlTuningParamsModule) waitForIntelTaskCompletion(taskID int) (*IntelTaskResult, error) {
	baseURL := strings.TrimSuffix(viper.GetString("defaults.api_url"), ".json")
	statusURL := fmt.Sprintf("%s/%d/", baseURL, taskID)
	token := viper.GetString("defaults.api_token")

	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Printf("⏳ Мониторинг задачи разведки %d", taskID)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("превышено время ожидания задачи %d", taskID)
		case <-ticker.C:
			status, err := m.getTaskStatus(statusURL, token, taskID)
			if err != nil {
				log.Printf("⚠️ Временная ошибка получения статуса задачи %d: %v", taskID, err)
				continue
			}

			if m.lastStatuses[taskID] != status.Status {
				log.Printf("🔄 Статус задачи разведки %d: %s", taskID, status.Status)
				m.lastStatuses[taskID] = status.Status
			}

			switch status.Status {
			case "completed", "success", "finished":
				log.Printf("✅ Разведка завершена успешно: %d", taskID)
				return status, nil
			case "failed", "canceled", "error":
				return nil, fmt.Errorf("разведочная задача %d завершилась с ошибкой", taskID)
			case "enqueued", "validating", "in_progress", "running", "performing":
				continue
			default:
				log.Printf("⚠️ Неизвестный статус задачи %d: %s", taskID, status.Status)
				continue
			}
		}
	}
}

func (m *PsqlTuningParamsModule) getTaskStatus(url, token string, taskID int) (*IntelTaskResult, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса статуса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса статуса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("разведочная задача не найдена")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("ошибка API (код %d): %s", resp.StatusCode, string(body))
	}

	body, _ := ioutil.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	intelResult := &IntelTaskResult{
		TaskID: taskID,
	}

	if status, ok := result["status"].(string); ok {
		intelResult.Status = status
	} else {
		return nil, fmt.Errorf("статус не найден в ответе")
	}

	if tableID, ok := result["table_id"].(string); ok {
		intelResult.TableID = tableID
	} else {
		intelResult.TableID = "psqlseclusterstandalone"
	}

	if completedAt, ok := result["completed_at"].(string); ok {
		intelResult.CompletedAt = completedAt
	}

	return intelResult, nil
}
