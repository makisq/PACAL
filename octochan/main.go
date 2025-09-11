package main

import (
	"fmt"
	"log"
	"octochan/cmd"
	"octochan/core"
	"octochan/module"
	"os"
	"strings"

	"github.com/fsnotify/fsnotify"
)

func init() {
	go watchModules()
}

func watchModules() {
	fm, err := core.NewFileManager()
	if err != nil {
		log.Printf("❌ Не удалось инициализировать FileManager: %v", err)
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("❌ Не удалось создать watcher: %v", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(fm.ModulesDir())
	if err != nil {
		log.Printf("❌ Не удалось добавить директорию для наблюдения: %v", err)
		return
	}

	log.Println("🔍 Наблюдение за изменениями модулей запущено")

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Create == fsnotify.Create {
				log.Printf("🔄 Обнаружено изменение модуля: %s", event.Name)
				module.AutoRegisterModules()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("❌ Ошибка watcher: %v", err)
		}
	}
}

func main() {
	defer core.GetModuleManager().Cleanup()

	args := os.Args[1:]
	if hasPipe(args) {
		if err := handlePipe(args); err != nil {
			fmt.Printf("❌ Pipe error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if hasPathArguments(args) {
		cmd.StartInteractiveShell()
	} else {
		cmd.Execute()
	}
}

func hasPipe(args []string) bool {
	for _, arg := range args {
		if arg == "|" {
			return true
		}
	}
	return false
}

func hasPathArguments(args []string) bool {
	for _, arg := range args {
		if arg == "cmd/" || arg == "core/" || arg == "module/" ||
			strings.Contains(arg, "/") || strings.Contains(arg, "\\") {
			return true
		}
	}
	return false
}

func handlePipe(args []string) error {
	commands := splitPipeCommands(args)
	if len(commands) == 0 {
		return fmt.Errorf("invalid pipe syntax")
	}

	var prevOutput []byte
	for i, cmdArgs := range commands {
		if len(cmdArgs) == 0 {
			continue
		}

		var inputData []byte
		if i > 0 {
			inputData = prevOutput
		}

		output, err := cmd.ExecuteCommandWithPipe(cmdArgs[0], cmdArgs[1:], inputData)
		if err != nil {
			return fmt.Errorf("command %s failed: %v", cmdArgs[0], err)
		}

		prevOutput = output
		if i == len(commands)-1 {
			fmt.Print(string(output))
		}
	}

	return nil
}

func splitPipeCommands(args []string) [][]string {
	var commands [][]string
	var currentCmd []string

	for _, arg := range args {
		if arg == "|" {
			if len(currentCmd) > 0 {
				commands = append(commands, currentCmd)
				currentCmd = []string{}
			}
		} else {
			currentCmd = append(currentCmd, arg)
		}
	}

	if len(currentCmd) > 0 {
		commands = append(commands, currentCmd)
	}

	return commands
}
