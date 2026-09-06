package cmd

import (
	"fmt"
	"os"

	"octochan/core"

	"github.com/spf13/cobra"
)

var (
	patchOutput string
	patchForce  bool
)

var patchCmd = &cobra.Command{
	Use:   "patch <base_file> <changes_file>",
	Short: "Применить изменения к основному конфигу",
	// Args раньше не задавались вовсе, поэтому `ochan patch` без аргументов
	// падал с index out of range.
	Args:    cobra.ExactArgs(2),
	Example: "ochan patch prod.conf changes.conf -o patched.conf",
	RunE: func(cmd *cobra.Command, args []string) error {
		base, err := readSource(args[0])
		if err != nil {
			return err
		}
		changes, err := readSource(args[1])
		if err != nil {
			return err
		}

		baseCfg, err := core.ParseConfig(base.Content)
		if err != nil {
			return fmt.Errorf("%s: %w", base.Name, err)
		}
		changesCfg, err := core.ParseConfig(changes.Content)
		if err != nil {
			return fmt.Errorf("%s: %w", changes.Name, err)
		}

		applied := 0
		for section, params := range changesCfg {
			if _, exists := baseCfg[section]; !exists {
				baseCfg[section] = make(map[string]string)
			}
			for key, value := range params {
				baseCfg[section][key] = value
				applied++
			}
		}

		// Путь вывода раньше был захардкожен как "patched.conf" в текущей
		// директории и молча перезаписывался.
		output := patchOutput
		if output == "" {
			output = "patched.conf"
		}
		if !patchForce {
			if _, err := os.Stat(output); err == nil {
				return fmt.Errorf("файл %s уже существует, используйте --force для перезаписи", output)
			}
		}

		if err := core.SaveConfig(baseCfg, output); err != nil {
			return err
		}

		cmd.Printf("Применено параметров: %d. Результат сохранён в %s\n", applied, output)
		return nil
	},
}

func init() {
	patchCmd.Flags().StringVarP(&patchOutput, "output", "o", "",
		"куда записать результат (по умолчанию patched.conf)")
	patchCmd.Flags().BoolVar(&patchForce, "force", false,
		"перезаписать существующий файл")
}
