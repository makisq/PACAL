package cmd

import (
	"fmt"
	"strings"

	"octochan/core"

	"github.com/spf13/cobra"
)

var planRLM bool

var planCmd = &cobra.Command{
	Use:   "plan <база> <изменения> | plan --rlm <сценарий> [key=value ...]",
	Short: "Показать, что произойдёт, ничего не делая",
	Long: "Для конфигов показывает, какие параметры изменит patch.\n" +
		"Для сценариев (--rlm) показывает, какие задачи были бы созданы\n" +
		"и на каких серверах.\n\n" +
		"В режиме плана изменяющие запросы к API запрещены на уровне клиента,\n" +
		"поэтому команда не может ничего создать даже по ошибке.",
	Args: cobra.MinimumNArgs(1),
	Example: "ochan plan prod.conf changes.conf\n" +
		"ochan plan эталон.conf postgres://dba@db1:5432/postgres\n" +
		"ochan plan --rlm restart.yaml",
	RunE: func(cmd *cobra.Command, args []string) error {
		if planRLM {
			return planScenario(cmd, args)
		}
		return planConfig(cmd, args)
	},
}

func planConfig(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("нужны два источника: база и изменения")
	}

	ctx := cmd.Context()
	opts := core.SourceOptions{}

	base, err := loadConfigArg(ctx, args[0], opts)
	if err != nil {
		return err
	}
	changes, err := loadConfigArg(ctx, args[1], opts)
	if err != nil {
		return err
	}

	diff := core.CompareConfigs(base.Data, changes.Data, base.Name, changes.Name)
	plan := core.PlanFromDiff(diff, base.Name)
	plan.Command = "patch"
	plan.Source = changes.Name

	plan.Warnings = append(plan.Warnings, base.Notes...)
	plan.Warnings = append(plan.Warnings, changes.Notes...)

	cmd.Print(plan.Render(60))
	return nil
}

func planScenario(cmd *cobra.Command, args []string) error {
	scenarioPath, err := argOrStdin(args, 0)
	if err != nil {
		return fmt.Errorf("укажите файл сценария: %w", err)
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

	client, err := core.NewAPIClient()
	if err != nil {
		return err
	}

	plan, err := core.PlanScenario(cmd.Context(), client, []byte(src.Content), customParams)
	if err != nil {
		return err
	}

	plan.Source = fmt.Sprintf("%s (%s)", src.Name, plan.Source)
	cmd.Print(plan.Render(60))

	if plan.Destructive {
		cmd.Println("\nДля выполнения: ochan apply --rlm " + strings.Join(args, " "))
	}
	return nil
}

func init() {
	planCmd.Flags().BoolVarP(&planRLM, "rlm", "r", false, "планировать RLM-сценарий")
}
