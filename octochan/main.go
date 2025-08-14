package main

import (
	"log"
	"octochan/cmd"
	"octochan/core"
	"octochan/module"

	"github.com/fsnotify/fsnotify"
)

func init() {
	go watchModules()
}

func watchModules() {
	fm, err := core.NewFileManager()
	if err != nil {
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	defer watcher.Close()

	err = watcher.Add(fm.ModulesDir())
	if err != nil {
		return
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Create == fsnotify.Create {
				module.AutoRegisterModules()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Ошибка watcher: %v", err)
		}
	}
}
func main() {
	defer core.GetModuleManager().Cleanup()
	cmd.StartInteractiveShell()
}
