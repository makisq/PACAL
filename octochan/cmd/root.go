package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ochan",
	Short: "Инструмент для детекта изменений в конфигурациях",
	Long: `Octo-chan — утилита для сравнения и анализа конфигурационных файлов.
Поддерживает YAML, JSON, ENV и другие форматы.
Находит различия, проверяет валидность и умеет применять патчи.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Welcome to octo-chan! Use 'help' for usage.")
	},
}

func init() {
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	rootCmd.AddCommand(diffCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(patchCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.SetHelpCommand(helpCmd)
	rootCmd.AddCommand(helpCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
