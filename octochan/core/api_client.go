package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type APIRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    map[string]interface{}
}

type ScenarioModule interface {
	Validate() error
	GenerateRequests() ([]*APIRequest, error)
}

type ScenarioData struct {
	Service    string                 `yaml:"service"`
	Parameters map[string]interface{} `yaml:"parameters"`
	Targets    []Target               `yaml:"targets"`
}

type Target struct {
	SVMCI string `yaml:"svm_ci"`
}

type Scenario struct {
	Service    string                 `yaml:"service"`
	Parameters map[string]interface{} `yaml:"parameters"`
	Targets    []Target               `yaml:"targets"`
	Items      []map[string]string    `yaml:"items"`
}

type scenarioModuleCreator func(data *ScenarioData) (ScenarioModule, error)

var scenarioModules = make(map[string]scenarioModuleCreator)

func RegisterScenarioModule(serviceName string, creator scenarioModuleCreator) {
	scenarioModules[serviceName] = creator
}

func ExecuteRequest(req *APIRequest) (string, error) {
	jsonData, err := json.Marshal(req.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка формирования JSON: %w", err)
	}

	httpReq, err := http.NewRequest(req.Method, req.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ошибка отправки запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ошибка API (%d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	switch v := result["id"].(type) {
	case string:
		return v, nil
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', 0, 64), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case nil:
		return "", fmt.Errorf("ID задачи отсутствует в ответе сервера")
	default:
		if id, ok := result["id"].(string); ok {
			return id, nil
		} else if idNum, ok := result["id"].(float64); ok {
			return fmt.Sprintf("%.0f", idNum), nil
		}
		return "", fmt.Errorf("неподдерживаемый формат ID: %T, полный ответ: %v", result["id"], result)
	}
}
func ParseScenarioData(data []byte) (*ScenarioData, error) {
	var jsonData struct {
		Service    string                 `json:"service"`
		Parameters map[string]interface{} `json:"parameters"`
		Targets    interface{}            `json:"targets"`
		Items      []map[string]string    `json:"items"`
	}

	if err := json.Unmarshal(data, &jsonData); err == nil {
		return parseScenarioDataFromInterface(jsonData.Service, jsonData.Parameters, jsonData.Targets)
	}

	var yamlData struct {
		Service    string                 `yaml:"service"`
		Parameters map[string]interface{} `yaml:"parameters"`
		Targets    interface{}            `yaml:"targets"`
		Items      []map[string]string    `yaml:"items"`
	}

	if err := yaml.Unmarshal(data, &yamlData); err != nil {
		return nil, fmt.Errorf("ошибка парсинга сценария (ни JSON, ни YAML): %w", err)
	}

	return parseScenarioDataFromInterface(yamlData.Service, yamlData.Parameters, yamlData.Targets)
}

func parseScenarioDataFromInterface(service string, parameters map[string]interface{}, targets interface{}) (*ScenarioData, error) {
	s := &ScenarioData{
		Service:    service,
		Parameters: parameters,
	}

	switch v := targets.(type) {
	case []interface{}:
		s.Targets = make([]Target, 0)
		for _, item := range v {
			if targetMap, ok := item.(map[string]interface{}); ok {
				if ciList, ok := targetMap["svm_ci"].([]interface{}); ok {
					for _, ci := range ciList {
						if ciStr, ok := ci.(string); ok {
							s.Targets = append(s.Targets, Target{SVMCI: ciStr})
						} else {
							return nil, fmt.Errorf("неверный формат svm_ci в списке: ожидается строка, получено %T", ci)
						}
					}
				} else if ciStr, ok := targetMap["svm_ci"].(string); ok {
					s.Targets = append(s.Targets, Target{SVMCI: ciStr})
				} else {
					return nil, fmt.Errorf("неверный формат target: ожидается svm_ci с массивом или строкой")
				}
			} else {
				return nil, fmt.Errorf("неверный формат target: ожидается объект с svm_ci")
			}
		}
	case []map[string]interface{}:
		s.Targets = make([]Target, 0)
		for _, t := range v {
			if ciList, ok := t["svm_ci"].([]interface{}); ok {
				for _, ci := range ciList {
					if ciStr, ok := ci.(string); ok {
						s.Targets = append(s.Targets, Target{SVMCI: ciStr})
					} else {
						return nil, fmt.Errorf("неверный формат svm_ci в списке: ожидается строка, получено %T", ci)
					}
				}
			} else if ci, ok := t["svm_ci"].(string); ok {
				s.Targets = append(s.Targets, Target{SVMCI: ci})
			} else {
				return nil, fmt.Errorf("неверный формат target: ожидается svm_ci с массивом или строкой")
			}
		}
	case map[string]interface{}:

		if ciList, ok := v["svm_ci"].([]interface{}); ok {
			s.Targets = make([]Target, 0)
			for _, ci := range ciList {
				if ciStr, ok := ci.(string); ok {
					s.Targets = append(s.Targets, Target{SVMCI: ciStr})
				} else {
					return nil, fmt.Errorf("неверный формат svm_ci в списке: ожидается строка, получено %T", ci)
				}
			}
		} else if ci, ok := v["svm_ci"].(string); ok {
			s.Targets = append(s.Targets, Target{SVMCI: ci})
		} else {
			return nil, fmt.Errorf("неверный формат targets: ожидается svm_ci с массивом или строкой")
		}
	default:
		return nil, fmt.Errorf("неподдерживаемый формат targets: %T", v)
	}

	if s.Service == "" {
		return nil, fmt.Errorf("не указано имя сервиса (service)")
	}

	if len(s.Targets) == 0 {
		return nil, fmt.Errorf("не указаны целевые серверы (targets)")
	}

	for i, target := range s.Targets {
		if target.SVMCI == "" {
			return nil, fmt.Errorf("пустой svm_ci в target #%d", i+1)
		}
	}

	return s, nil
}
func (s *Scenario) GetServers() []string {
	servers := make([]string, 0, len(s.Targets))
	for _, target := range s.Targets {
		servers = append(servers, target.SVMCI)
	}
	return servers
}

func prepareItemsWithTarget(target Target) map[string]string {
	return map[string]string{
		"table_id":      "psqlseclusterstandalone",
		"invsvm_ci_svm": target.SVMCI,
	}
}

func ExecuteScenario(data []byte) ([]string, error) {

	scenario, err := ParseScenarioData(data)
	if err != nil {
		return nil, err
	}

	creator, exists := scenarioModules[scenario.Service]
	if !exists {
		return nil, fmt.Errorf("модуль для сервиса '%s' не найден", scenario.Service)
	}

	module, err := creator(scenario)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания модуля: %w", err)
	}

	if err := module.Validate(); err != nil {
		return nil, fmt.Errorf("ошибка валидации: %w", err)
	}

	requests, err := module.GenerateRequests()
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации запросов: %w", err)
	}

	var taskIDs []string
	for _, req := range requests {
		taskID, err := ExecuteRequest(req)
		if err != nil {
			return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
		}
		taskIDs = append(taskIDs, taskID)
	}

	return taskIDs, nil
}

func CheckTaskStatus(taskID string) (string, error) {
	apiURL := fmt.Sprintf("%s/%s/", viper.GetString("defaults.api_url"), taskID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+viper.GetString("defaults.api_token"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ошибка запроса статуса: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ошибка API (код %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	if status, ok := result["status"].(string); ok {
		return status, nil
	}

	return "", fmt.Errorf("не удалось получить статус задачи")
}

func ExecuteRequestWithRetry(req *APIRequest, retries int) (string, error) {
	var lastErr error

	for i := 0; i < retries; i++ {
		taskID, err := ExecuteRequest(req)
		if err == nil {
			return taskID, nil
		}
		lastErr = err
		time.Sleep(time.Second * time.Duration(i+1))
	}
	return "", fmt.Errorf("после %d попыток: %w", retries, lastErr)
}

func ApplyScenario(path string, customParams map[string]string) ([]string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла: %w", err)
	}
	return ExecuteModularScenario(data, customParams)
}

func ExecuteModularScenario(scenarioData []byte, customParams map[string]string) ([]string, error) {
	if len(scenarioData) == 0 {
		return nil, fmt.Errorf("пустые данные сценария")
	}
	data, err := ParseScenarioData(scenarioData)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга сценария: %w", err)
	}
	if data.Parameters == nil {
		data.Parameters = make(map[string]interface{})
	}
	for key, val := range customParams {
		data.Parameters[key] = val
	}

	creator, exists := scenarioModules[data.Service]
	if !exists {
		availableModules := make([]string, 0, len(scenarioModules))
		for moduleName := range scenarioModules {
			availableModules = append(availableModules, moduleName)
		}
		return nil, fmt.Errorf(
			"модуль для сервиса '%s' не зарегистрирован. Доступные модули: %v",
			data.Service,
			availableModules,
		)
	}

	module, err := creator(data)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания модуля: %w", err)
	}
	if err := module.Validate(); err != nil {
		return nil, fmt.Errorf("ошибка валидации: %w", err)
	}
	requests, err := module.GenerateRequests()
	if err != nil {
		return nil, fmt.Errorf("ошибка подготовки запросов: %w", err)
	}

	maxParallel := viper.GetInt("defaults.max_parallel_tasks")
	if maxParallel <= 0 {
		maxParallel = 5
	}

	var wg sync.WaitGroup
	taskIDs := make([]string, 0, len(requests))
	taskErrors := make(chan error, len(requests))
	results := make(chan string, len(requests))
	semaphore := make(chan struct{}, maxParallel)

	for _, req := range requests {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(r *APIRequest) {
			defer func() {
				<-semaphore
				wg.Done()
			}()

			taskID, err := ExecuteRequestWithRetry(r, 3)
			if err != nil {
				taskErrors <- fmt.Errorf("ошибка выполнения запроса: %w", err)
				return
			}

			results <- taskID
			fmt.Printf("✅ Задача создана: %s\n", taskID)
			go func(id string) {
				if err := monitorTaskStatus(id); err != nil {
					fmt.Printf("⚠️ Ошибка мониторинга задачи %s: %v\n", id, err)
				}
			}(taskID)
		}(req)
	}
	go func() {
		wg.Wait()
		close(results)
		close(taskErrors)
	}()
	for taskID := range results {
		taskIDs = append(taskIDs, taskID)
	}
	select {
	case err := <-taskErrors:
		return taskIDs, err
	default:
		return taskIDs, nil
	}
}
func monitorTaskStatus(taskID string) error {
	fmt.Printf("\n🔄 Начинаем мониторинг задачи %s...\n", taskID)

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	var lastStatus string
	for {
		select {
		case <-ticker.C:
			status, err := GetTaskStatusWithEvents(taskID)
			if err != nil {
				return fmt.Errorf("ошибка получения статуса: %w", err)
			}

			if taskStatus, ok := status["status"].(string); ok {
				if taskStatus != lastStatus {
					fmt.Printf("Текущий статус: %s\n", taskStatus)
					lastStatus = taskStatus
				}

				switch taskStatus {
				case "completed", "failed", "canceled", "success":
					fmt.Printf("Завершено с статусом: %s\n", taskStatus)
					return nil
				}
			}

		case <-time.After(30 * time.Minute):
			return fmt.Errorf("превышено время ожидания выполнения задачи")
		}
	}
}
func PrepareScenarioItems(targets []Target) ([]map[string]string, error) {
	var items []map[string]string
	for _, target := range targets {
		if target.SVMCI == "" {
			return nil, fmt.Errorf("пустой SVMCI в target")
		}
		items = append(items, prepareItemsWithTarget(target))
	}
	return items, nil
}

func DeepCopyMap(m map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})
	for k, v := range m {
		if vm, ok := v.(map[string]interface{}); ok {
			copy[k] = DeepCopyMap(vm)
		} else {
			copy[k] = v
		}
	}
	return copy
}

func GetTaskStatusWithEvents(taskID string) (map[string]interface{}, error) {
	baseURL := strings.TrimSuffix(viper.GetString("defaults.api_url"), ".json")
	baseURL = strings.TrimSuffix(baseURL, "/")

	statusURL := fmt.Sprintf("%s/%s/", baseURL, taskID)
	eventsURL := fmt.Sprintf("%s/%s/events/", baseURL, taskID)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	result, err := getTaskStatus(client, statusURL)
	if err != nil {
		return nil, err
	}
	if events, err := getTaskEvents(client, eventsURL); err == nil {
		result["events"] = events
	} else if !strings.Contains(err.Error(), "404") {
		fmt.Printf("⚠️ Не удалось получить события: %v\n", err)
	}

	return result, nil
}

func getTaskStatus(client *http.Client, url string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+viper.GetString("defaults.api_token"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка HTTP запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка API (код %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	return result, nil
}

func getTaskEvents(client *http.Client, url string) ([]map[string]interface{}, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+viper.GetString("defaults.api_token"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка HTTP запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("ошибка API (код %d): %s", resp.StatusCode, string(body))
	}

	var events []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	return events, nil
}

func PrintTaskStatus(taskID string) error {
	status, err := GetTaskStatusWithEvents(taskID)
	if err != nil {
		return fmt.Errorf("ошибка получения статуса задачи %s: %w", taskID, err)
	}

	fmt.Printf("\n📊 Статус задачи %s\n", taskID)
	fmt.Println("----------------------------")

	if val, ok := status["status"].(string); ok {
		fmt.Printf("Состояние: %s\n", val)
	}
	if val, ok := status["created_at"].(string); ok {
		fmt.Printf("Создана: %s\n", val)
	}
	if val, ok := status["updated_at"].(string); ok {
		fmt.Printf("Обновлена: %s\n", val)
	}
	if val, ok := status["progress"].(float64); ok {
		fmt.Printf("Прогресс: %.0f%%\n", val)
	}
	if events, ok := status["events"].([]map[string]interface{}); ok {
		fmt.Println("\n📝 Логи выполнения:")
		for _, event := range events {
			if timestamp, ok := event["timestamp"].(string); ok {
				if level, ok := event["level"].(string); ok {
					if message, ok := event["message"].(string); ok {
						fmt.Printf("[%s] %s - %s\n", timestamp, level, message)
					}
				}
			}
		}
	}

	return nil
}

func GetTaskStatus(taskID string) (map[string]interface{}, error) {
	apiURL := fmt.Sprintf("%s/%s/", viper.GetString("defaults.api_url"), taskID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+viper.GetString("defaults.api_token"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка API (код %d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	return result, nil
}

func GetTaskLogs(taskID string) ([]map[string]interface{}, error) {
	apiURL := fmt.Sprintf("%s/%s/events/", viper.GetString("defaults.api_url"), taskID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Token "+viper.GetString("defaults.api_token"))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка API (код %d): %s", resp.StatusCode, string(body))
	}

	var logs []map[string]interface{}
	if err := json.Unmarshal(body, &logs); err != nil {
		return nil, fmt.Errorf("ошибка парсинга логов: %w", err)
	}

	return logs, nil
}
