package rlm

import (
	"context"
	"fmt"
)

// dryRunAPI пропускает чтение и отклоняет любую попытку что-то создать.
//
// Раньше режим плана был изменяемым полем на клиенте (SetDryRun). Обёртка
// лучше по двум причинам: у клиента не остаётся состояния, которое можно
// забыть сбросить, и модуль физически не может добраться до настоящего Post —
// он держит только интерфейс, а за интерфейсом стоит эта обёртка.
type dryRunAPI struct {
	inner ScenarioAPI
}

// DryRun возвращает представление API, в котором запрещены изменения.
func DryRun(api ScenarioAPI) ScenarioAPI {
	if api == nil {
		return nil
	}
	if _, already := api.(dryRunAPI); already {
		return api
	}
	return dryRunAPI{inner: api}
}

func (d dryRunAPI) TasksURL() string { return d.inner.TasksURL() }
func (d dryRunAPI) IsDryRun() bool   { return true }

func (d dryRunAPI) Get(ctx context.Context, url string, out any) error {
	return d.inner.Get(ctx, url, out)
}

func (d dryRunAPI) Post(ctx context.Context, url string, _, _ any) error {
	return fmt.Errorf("%w (POST %s)", ErrDryRun, url)
}

// WaitForTask в режиме плана бессмысленно: задачи, которую можно ждать,
// не создавалось. Отказ здесь понятнее, чем зависание на опросе.
func (d dryRunAPI) WaitForTask(_ context.Context, taskID string, _ WaitOptions) (map[string]any, error) {
	return nil, fmt.Errorf("%w (ожидание задачи %s)", ErrDryRun, taskID)
}
