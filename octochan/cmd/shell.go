package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"octochan/pluginhost"

	"github.com/c-bata/go-prompt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Запустить интерактивную оболочку",
	Args:  cobra.NoArgs,
}

var commandHistory []string

func init() {
	shellCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runInteractiveShell(cmd.Context())
	}
}

// runInteractiveShell запускает REPL.
//
// Ручного управления терминалом здесь больше нет. Прежняя версия сама
// дёргала ioctl TCGETS/TCSETS через syscall — из-за этого проект собирался
// только под Linux, — а затем передавала терминал go-prompt, который
// выставляет raw-режим самостоятельно. Отсюда же росли `stty sane`
// и \r перед каждой строкой вывода.
func runInteractiveShell(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := startModuleWatcher(ctx); err != nil {
		fmt.Printf("⚠️ Горячая перезагрузка модулей недоступна: %v\n", err)
	}

	startTokenReminder(func(msg string) { fmt.Println("\n" + msg) })
	defer stopTokenReminder()

	fmt.Println("🐙 Добро пожаловать в OctoChan! Введите 'help' для списка команд, 'exit' для выхода.")

	p := prompt.New(
		executeShellInput,
		completer,
		prompt.OptionPrefix("ochan> "),
		prompt.OptionTitle("OctoChan Interactive Shell"),
		prompt.OptionAddKeyBind(prompt.KeyBind{
			Key: prompt.ControlD,
			Fn:  func(*prompt.Buffer) { exitShell() },
		}),
	)
	p.Run()
	return nil
}

// exitShell завершает процесс из оболочки.
//
// go-prompt не умеет прерывать Run() изнутри executor'а, поэтому выход —
// это os.Exit. Значит, defer'ы из Execute не отработают, и процессы плагинов
// нужно погасить здесь явно: прежняя версия этого не делала, и после выхода
// из оболочки они оставались висеть.
func exitShell() {
	stopTokenReminder()
	pluginhost.GetModuleManager().Cleanup()
	os.Exit(0)
}

// executeShellInput выполняет одну строку оболочки.
func executeShellInput(input string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	// Перезагрузка модулей применяется здесь, в главной горутине,
	// а не в горутине наблюдателя.
	applyModuleChanges(func(msg string) { fmt.Println(msg) })

	switch input {
	case "exit", "quit":
		fmt.Println("Goodbye! 👋")
		// go-prompt перехватывает Ctrl-D/Ctrl-C сам; выход выполняется
		// через panic-free путь os.Exit в вызывающем коде.
		exitShell()
		return
	case "!clear", "clr":
		fmt.Print("\x1b[2J\x1b[H")
		return
	case "!history", "his":
		showCommandHistory()
		return
	}

	if !strings.HasPrefix(input, "!") {
		commandHistory = append(commandHistory, input)
	}

	args, err := splitArgs(input)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return
	}

	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("❌ %v\n", err)
	}
}

// splitArgs разбивает строку на аргументы с учётом кавычек.
// Прежний strings.Fields ломал аргументы с пробелами, из-за чего
// `if config.conf port == 8080 "Порт корректен"` превращался в семь
// аргументов вместо пяти и падал на проверке ExactArgs(5).
func splitArgs(input string) ([]string, error) {
	var (
		args    []string
		current strings.Builder
		quote   rune
		hasWord bool
	)

	for _, r := range input {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			hasWord = true
		case r == ' ' || r == '\t':
			if hasWord {
				args = append(args, current.String())
				current.Reset()
				hasWord = false
			}
		default:
			current.WriteRune(r)
			hasWord = true
		}
	}

	if quote != 0 {
		return nil, fmt.Errorf("незакрытая кавычка %c", quote)
	}
	if hasWord {
		args = append(args, current.String())
	}
	return args, nil
}

func showCommandHistory() {
	if len(commandHistory) == 0 {
		fmt.Println("История пуста")
		return
	}

	fmt.Println("История команд:")
	for i, cmd := range commandHistory {
		fmt.Printf("%3d. %s\n", i+1, cmd)
	}
}

// ---------------------------------------------------------------------------
// Автодополнение
// ---------------------------------------------------------------------------

func completer(d prompt.Document) []prompt.Suggest {
	args := strings.Fields(d.TextBeforeCursor())
	if len(args) <= 1 && !strings.HasSuffix(d.TextBeforeCursor(), " ") {
		return completeCommands(d)
	}

	target, _, err := rootCmd.Find(args)
	if err != nil || target == nil {
		return nil
	}

	if strings.HasPrefix(d.GetWordBeforeCursor(), "-") {
		return completeFlags(target, d)
	}

	// Подкоманды текущей команды.
	var suggests []prompt.Suggest
	for _, sub := range target.Commands() {
		if sub.Hidden {
			continue
		}
		suggests = append(suggests, prompt.Suggest{Text: sub.Name(), Description: sub.Short})
	}
	return prompt.FilterHasPrefix(suggests, d.GetWordBeforeCursor(), true)
}

func completeCommands(d prompt.Document) []prompt.Suggest {
	var suggests []prompt.Suggest
	for _, c := range rootCmd.Commands() {
		if c.Hidden {
			continue
		}
		suggests = append(suggests, prompt.Suggest{Text: c.Name(), Description: c.Short})
	}

	suggests = append(suggests,
		prompt.Suggest{Text: "exit", Description: "Выйти из оболочки"},
		prompt.Suggest{Text: "!clear", Description: "Очистить экран"},
		prompt.Suggest{Text: "!history", Description: "История команд"},
	)

	return prompt.FilterHasPrefix(suggests, d.GetWordBeforeCursor(), true)
}

func completeFlags(cmd *cobra.Command, d prompt.Document) []prompt.Suggest {
	var suggests []prompt.Suggest

	add := func(flag *pflag.Flag) {
		suggests = append(suggests, prompt.Suggest{Text: "--" + flag.Name, Description: flag.Usage})
		if flag.Shorthand != "" {
			suggests = append(suggests, prompt.Suggest{Text: "-" + flag.Shorthand, Description: flag.Usage})
		}
	}

	cmd.LocalFlags().VisitAll(add)
	cmd.InheritedFlags().VisitAll(add)

	return prompt.FilterHasPrefix(suggests, d.GetWordBeforeCursor(), false)
}
