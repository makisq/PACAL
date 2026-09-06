package cmd

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"octochan/config"
	"octochan/rlm"
	"octochan/workspace"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	useRLM   bool
	waitDone bool
)

var applyCmd = &cobra.Command{
	Use:   "apply [--rlm] <file|-> [key=value ...]",
	Short: "Применить сценарий или конфигурацию",
	Args:  cobra.MinimumNArgs(0),
	Example: "ochan apply --rlm scenario.yaml role=standalone\n" +
		"cat scenario.yaml | ochan apply --rlm -",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !useRLM {
			return applyPlainConfig(cmd, args)
		}
		return applyRLMScenario(cmd, args)
	},
}

func applyPlainConfig(cmd *cobra.Command, args []string) error {
	path, err := argOrStdin(args, 0)
	if err != nil {
		return fmt.Errorf("для обычного применения укажите файл конфигурации: %w", err)
	}

	src, err := readSource(path)
	if err != nil {
		return err
	}
	cfg, err := config.ParseConfig(src.Content)
	if err != nil {
		return fmt.Errorf("%s: %w", src.Name, err)
	}

	total := 0
	for _, params := range cfg {
		total += len(params)
	}
	cmd.Printf("Разобран конфиг %s: секций %d, параметров %d\n", src.Name, len(cfg), total)
	return nil
}

func applyRLMScenario(cmd *cobra.Command, args []string) error {
	// Первый аргумент — файл сценария (или "-"), остальные — переопределения
	// параметров. Прежняя версия при пустом args безусловно читала stdin
	// и на терминале зависала навсегда.
	scenarioPath, err := argOrStdin(args, 0)
	if err != nil {
		return fmt.Errorf("укажите файл сценария или подайте его на stdin: %w", err)
	}

	overrides := args
	if len(args) > 0 {
		overrides = args[1:]
	}
	customParams, err := parseCustomParams(overrides)
	if err != nil {
		return err
	}

	src, err := readSource(scenarioPath)
	if err != nil {
		return err
	}

	client, err := rlm.NewAPIClient()
	if err != nil {
		return err
	}

	log.Printf("apply: запуск сценария %s", src.Name)

	ctx := cmd.Context()
	taskIDs, err := rlm.ExecuteScenario(ctx, client, []byte(src.Content), customParams)

	if len(taskIDs) > 0 {
		cmd.Println("✅ Созданные задачи:")
		for _, taskID := range taskIDs {
			cmd.Printf("  - %s\n", taskID)
		}

		// Задачи во внешней системе необратимы: перезапуск уже произошёл,
		// и отменять в октохане нечего. Но след остаться обязан.
		record(cmd, &workspace.Entry{
			Kind:       workspace.EntryApply,
			Command:    "apply --rlm",
			Source:     src.Name,
			Summary:    fmt.Sprintf("создано задач: %d", len(taskIDs)),
			TaskIDs:    taskIDs,
			Reversible: false,
		})
	}
	if err != nil {
		return err
	}

	// Мониторинг выполняется здесь и синхронно. Раньше core запускал его
	// в фоновой горутине, за которой никто не ждал: процесс завершался
	// сразу после создания задач, и статус никогда не печатался.
	if !waitDone && !viper.GetBool("defaults.auto_check_status") {
		return nil
	}

	cmd.Println("\n🔄 Ожидание завершения задач...")
	for _, taskID := range taskIDs {
		status, err := client.WaitForTask(ctx, taskID, rlm.WaitOptions{
			OnStatus: func(s string) { cmd.Printf("  %s: %s\n", taskID, s) },
		})
		if err != nil {
			return fmt.Errorf("задача %s: %w", taskID, err)
		}
		cmd.Printf("  %s: завершена (%v)\n", taskID, status["status"])
	}
	return nil
}

func parseCustomParams(args []string) (map[string]string, error) {
	if len(args) == 0 {
		return nil, nil
	}

	params := make(map[string]string, len(args))
	for _, arg := range args {
		key, value, found := strings.Cut(arg, "=")
		if !found || key == "" {
			// Раньше некорректный параметр просто писался в лог и терялся.
			return nil, fmt.Errorf("неверный формат параметра %q, ожидается key=value", arg)
		}
		params[key] = value
	}
	return params, nil
}

var statusCmd = &cobra.Command{
	Use:   "status <task_id>",
	Short: "Проверить статус выполнения задачи",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := rlm.NewAPIClient()
		if err != nil {
			return err
		}

		taskID := args[0]
		status, err := client.TaskStatusWithEvents(cmd.Context(), taskID)
		if err != nil {
			return err
		}

		cmd.Printf("\n📊 Статус задачи %s\n", taskID)
		cmd.Println("----------------------------")
		printField(cmd, status, "status", "Состояние")
		printField(cmd, status, "created_at", "Создана")
		printField(cmd, status, "updated_at", "Обновлена")
		if progress, ok := status["progress"].(float64); ok {
			cmd.Printf("Прогресс: %.0f%%\n", progress)
		}

		events, ok := status["events"].([]map[string]any)
		if !ok || len(events) == 0 {
			return nil
		}

		cmd.Println("\n📝 Логи выполнения:")
		for _, event := range events {
			cmd.Printf("[%v] %v - %v\n", event["timestamp"], event["level"], event["message"])
		}
		return nil
	},
}

func printField(cmd *cobra.Command, data map[string]any, key, label string) {
	if value, ok := data[key]; ok && value != nil {
		cmd.Printf("%s: %v\n", label, value)
	}
}

var saveScenarioCmd = &cobra.Command{
	Use:   "srs <scenario.yaml>",
	Short: "Сохранить сценарий во внутреннее хранилище",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fm, err := mustFileManager()
		if err != nil {
			return err
		}

		src, err := readSource(args[0])
		if err != nil {
			return err
		}

		// Проверяем сценарий до сохранения, чтобы в хранилище не оседал мусор.
		if _, err := rlm.ParseScenarioData([]byte(src.Content)); err != nil {
			return fmt.Errorf("сценарий не прошёл проверку: %w", err)
		}

		name := strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0]))
		path, err := fm.SaveScenario(name, []byte(src.Content))
		if err != nil {
			return err
		}

		// Путь берётся из того же места, откуда его потом прочитает
		// LoadScenario: раньше здесь печатался третий, неверный вариант.
		cmd.Printf("✅ Сценарий сохранён: %s\n", path)
		return nil
	},
}

func init() {
	applyCmd.Flags().BoolVarP(&useRLM, "rlm", "r", false, "выполнить как RLM-сценарий")
	applyCmd.Flags().BoolVarP(&waitDone, "wait", "w", false, "дождаться завершения задач")
}
