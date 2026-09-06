package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TaskState — обобщённое состояние задачи RLM.
type TaskState int

const (
	TaskStateRunning TaskState = iota
	TaskStateSucceeded
	TaskStateFailed
	TaskStateUnknown
)

// Раньше словарь статусов был продублирован в core и в module с разными
// наборами значений, из-за чего одна и та же задача считалась завершённой
// в одном месте и «неизвестной» в другом.
var (
	succeededStatuses = map[string]bool{
		"completed": true,
		"success":   true,
		"succeeded": true,
		"finished":  true,
		"done":      true,
	}
	failedStatuses = map[string]bool{
		"failed":    true,
		"canceled":  true,
		"cancelled": true,
		"error":     true,
		"aborted":   true,
	}
	runningStatuses = map[string]bool{
		"enqueued":    true,
		"queued":      true,
		"validating":  true,
		"in_progress": true,
		"running":     true,
		"performing":  true,
		"pending":     true,
		"new":         true,
	}
)

// TaskStateOf классифицирует строковый статус задачи.
func TaskStateOf(status string) TaskState {
	s := strings.ToLower(strings.TrimSpace(status))
	switch {
	case succeededStatuses[s]:
		return TaskStateSucceeded
	case failedStatuses[s]:
		return TaskStateFailed
	case runningStatuses[s]:
		return TaskStateRunning
	default:
		return TaskStateUnknown
	}
}

// WaitOptions настраивает опрос статуса задачи.
type WaitOptions struct {
	// PollInterval — период опроса; 0 означает значение по умолчанию.
	PollInterval time.Duration
	// Timeout — предельное время ожидания; 0 означает значение по умолчанию.
	Timeout time.Duration
	// OnStatus вызывается при каждой смене статуса.
	OnStatus func(status string)
}

func (o WaitOptions) pollInterval() time.Duration {
	if o.PollInterval > 0 {
		return o.PollInterval
	}
	return 10 * time.Second
}

func (o WaitOptions) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return 30 * time.Minute
}

// TaskStatus возвращает текущее состояние задачи.
func (c *APIClient) TaskStatus(ctx context.Context, taskID string) (map[string]any, error) {
	var result map[string]any
	if err := c.Get(ctx, c.TaskURL(taskID), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// TaskEvents возвращает журнал событий задачи.
func (c *APIClient) TaskEvents(ctx context.Context, taskID string) ([]map[string]any, error) {
	var events []map[string]any
	if err := c.Get(ctx, c.TaskEventsURL(taskID), &events); err != nil {
		return nil, err
	}
	return events, nil
}

// TaskStatusWithEvents возвращает статус задачи вместе с журналом событий.
// Отсутствие журнала (404) не считается ошибкой.
func (c *APIClient) TaskStatusWithEvents(ctx context.Context, taskID string) (map[string]any, error) {
	status, err := c.TaskStatus(ctx, taskID)
	if err != nil {
		return nil, err
	}

	events, err := c.TaskEvents(ctx, taskID)
	switch {
	case err == nil:
		status["events"] = events
	case IsNotFound(err):
		// журнала для этой задачи просто нет
	default:
		return status, fmt.Errorf("статус получен, но события недоступны: %w", err)
	}

	return status, nil
}

// WaitForTask опрашивает задачу до достижения терминального состояния.
//
// Таймаут задаётся одним таймером на весь цикл. В прежней версии
// `case <-time.After(...)` стоял внутри for-select, поэтому таймер
// пересоздавался на каждом тике и общий дедлайн не наступал никогда.
func (c *APIClient) WaitForTask(ctx context.Context, taskID string, opts WaitOptions) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	ticker := time.NewTicker(opts.pollInterval())
	defer ticker.Stop()

	var (
		lastStatus string
		lastErr    error
	)

	for {
		status, err := c.TaskStatus(ctx, taskID)
		if err != nil {
			// Разовый сбой сети не должен ронять ожидание, но и молча
			// теряться он тоже не должен: последнюю ошибку вернём по таймауту.
			lastErr = err
		} else {
			current, _ := status["status"].(string)
			if current != lastStatus {
				lastStatus = current
				if opts.OnStatus != nil {
					opts.OnStatus(current)
				}
			}

			switch TaskStateOf(current) {
			case TaskStateSucceeded:
				return status, nil
			case TaskStateFailed:
				return status, fmt.Errorf("задача %s завершилась со статусом %q", taskID, current)
			}
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("превышено время ожидания задачи %s (последняя ошибка: %v)", taskID, lastErr)
			}
			return nil, fmt.Errorf("превышено время ожидания задачи %s (последний статус: %q)", taskID, lastStatus)
		case <-ticker.C:
		}
	}
}
