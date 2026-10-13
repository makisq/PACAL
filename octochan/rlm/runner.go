package rlm

import (
	"context"
	"fmt"

	"octochan/config"
	"octochan/scenario"
)

// runner выполняет сценарий через RLM API.
//
// Это обычный scenario.Runner, зарегистрированный под именами своих сервисов.
// Команда apply не знает, что за ним стоит RLM: она спрашивает исполнителя
// у реестра по имени сервиса. Рядом может встать исполнитель, работающий
// по SSH, локально или как угодно ещё.
type runner struct{}

// Kind сообщает, каким способом будет выполнен сценарий.
func (runner) Kind() string { return "RLM" }

func (r runner) Plan(ctx context.Context, s *scenario.Scenario) (*config.Plan, error) {
	api, err := NewAPIClient()
	if err != nil {
		return nil, err
	}
	return PlanScenario(ctx, api, s)
}

func (r runner) Apply(ctx context.Context, s *scenario.Scenario) (*scenario.Result, error) {
	api, err := NewAPIClient()
	if err != nil {
		return nil, err
	}

	taskIDs, err := ExecuteScenario(ctx, api, s)

	result := &scenario.Result{
		Refs:    taskIDs,
		Summary: fmt.Sprintf("создано задач: %d", len(taskIDs)),
	}
	// Созданные задачи возвращаются даже при ошибке: они уже существуют
	// во внешней системе, и умолчать о них нельзя.
	return result, err
}

// Wait дожидается завершения задачи RLM.
// Реализует необязательный интерфейс scenario.Waiter.
func (r runner) Wait(ctx context.Context, ref string) (string, error) {
	api, err := NewAPIClient()
	if err != nil {
		return "", err
	}

	status, err := api.WaitForTask(ctx, ref, WaitOptions{})
	if err != nil {
		return "", err
	}

	state, _ := status["status"].(string)
	return state, nil
}

// RegisterScenarioModule регистрирует модуль сервиса RLM.
//
// Помимо внутреннего реестра модулей сервис объявляется в общем реестре
// исполнителей, чтобы apply нашёл его по имени без всяких флагов.
func RegisterScenarioModule(serviceName string, creator ScenarioModuleCreator) {
	scenarioModulesMu.Lock()
	scenarioModules[serviceName] = creator
	scenarioModulesMu.Unlock()

	scenario.Register(serviceName, runner{})
}
