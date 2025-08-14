package main

import (
	"fmt"
	"octochan/core"
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/spf13/cobra"
)

// TestModule реализует интерфейс CommandModule
type TestModule struct{}

func (t *TestModule) GetCommands() []*cobra.Command {
	return []*cobra.Command{
		{
			Use:   "test",
			Short: "Тестовая команда модуля",
			Run: func(cmd *cobra.Command, args []string) {
				cmd.Println("Я тестовый модуль")
			},
		},
	}
}

func (t *TestModule) Init(config map[string]interface{}) error {
	// Простая обработка конфигурации
	if val, ok := config["debug"]; ok {
		if debug, ok := val.(bool); ok && debug {
			fmt.Println("Debug mode enabled")
		}
	}
	return nil
}

func (t *TestModule) Name() string {
	return "test-module"
}

func (t *TestModule) Version() string {
	return "1.0.0"
}

var Module = &TestModule{}

func main() {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "test-module",
		Output: os.Stderr,
		Level:  hclog.Debug,
	})

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "OCTOCHAN_PLUGIN",
			MagicCookieValue: "octochan-2025",
		},
		Plugins: map[string]plugin.Plugin{
			"command": &core.CommandPlugin{Impl: Module},
		},
		Logger: logger,
	})
}
