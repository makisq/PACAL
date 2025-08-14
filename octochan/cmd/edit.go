package cmd

import (
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func completeFlags(cmd *cobra.Command, d prompt.Document) []prompt.Suggest {
	var suggests []prompt.Suggest

	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		suggests = append(suggests, prompt.Suggest{
			Text:        "--" + flag.Name,
			Description: flag.Usage,
		})

		if flag.Shorthand != "" {
			suggests = append(suggests, prompt.Suggest{
				Text:        "-" + flag.Shorthand,
				Description: flag.Usage,
			})
		}
	})

	cmd.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		suggests = append(suggests, prompt.Suggest{
			Text:        "--" + flag.Name,
			Description: flag.Usage,
		})

		if flag.Shorthand != "" {
			suggests = append(suggests, prompt.Suggest{
				Text:        "-" + flag.Shorthand,
				Description: flag.Usage,
			})
		}
	})

	return prompt.FilterHasPrefix(suggests, d.GetWordBeforeCursor(), false)
}

func completer(d prompt.Document) []prompt.Suggest {
	args := strings.Fields(d.TextBeforeCursor())
	if len(args) <= 1 {
		return completeCommands(d)
	}

	cmd, _, err := rootCmd.Find(args)
	if err != nil {
		return []prompt.Suggest{}
	}

	if strings.HasPrefix(d.GetWordBeforeCursor(), "-") {
		return completeFlags(cmd, d)
	}

	switch cmd.Name() {
	case "apply":
		return []prompt.Suggest{
			{Text: "scenario.yaml", Description: "RLM scenario file"},
		}
	case "diff":
		return []prompt.Suggest{
			{Text: "file1.yaml", Description: "First config file"},
			{Text: "file2.yaml", Description: "Second config file"},
		}
	}

	return []prompt.Suggest{}
}

func completeCommands(d prompt.Document) []prompt.Suggest {
	var suggests []prompt.Suggest

	for _, cmd := range rootCmd.Commands() {
		suggests = append(suggests, prompt.Suggest{
			Text:        cmd.Name(),
			Description: cmd.Short,
		})
	}

	suggests = append(suggests, []prompt.Suggest{
		{Text: "exit", Description: "Exit the shell"},
		{Text: "help", Description: "Show help"},
		{Text: "!clear", Description: "Clear screen"},
		{Text: "!history", Description: "Show command history"},
	}...)

	return prompt.FilterHasPrefix(suggests, d.GetWordBeforeCursor(), true)
}
