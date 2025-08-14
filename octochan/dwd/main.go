package main

import (
	"context"
	"fmt"
	"octochan/core"
	"os"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
)

type TestModule struct{}

func (t *TestModule) GetCommands() []*cobra.Command {
	// Основная команда test
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test command from module",
		Run: func(cmd *cobra.Command, args []string) {
			debug, _ := cmd.Flags().GetBool("debug")
			if debug {
				fmt.Println("Debug mode is ON")
			}
			fmt.Println("Test command executed successfully!")
		},
	}

	// Добавляем флаг debug к основной команде
	testCmd.Flags().BoolP("debug", "d", false, "Enable debug mode")

	// Подкоманда generate
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate test data",
		Run: func(cmd *cobra.Command, args []string) {
			count, _ := cmd.Flags().GetInt("count")
			fmt.Printf("Generating %d test items...\n", count)
			for i := 1; i <= count; i++ {
				fmt.Printf("Item %d\n", i)
			}
		},
	}

	// Добавляем флаг count к подкоманде generate
	generateCmd.Flags().IntP("count", "n", 3, "Number of items to generate")

	// Добавляем подкоманду к основной команде
	testCmd.AddCommand(generateCmd)

	return []*cobra.Command{testCmd}
}

func (t *TestModule) Init(config map[string]interface{}) error {
	return nil
}

func (t *TestModule) Name() string {
	return "test-module"
}

func (t *TestModule) Version() string {
	return "1.3.0"
}

type TestPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	Impl *TestModule
}

type CommandGRPCServer struct {
	core.UnimplementedCommandModuleServer
	Impl *TestModule
}

func (s *CommandGRPCServer) GetCommands(ctx context.Context, req *core.Empty) (*core.CommandsResponse, error) {
	commands := s.Impl.GetCommands()
	pbCommands := make([]*core.Command, len(commands))

	for i, cmd := range commands {
		var pbFlags []*core.Flag
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			pbFlag := &core.Flag{
				Name:      flag.Name,
				Shorthand: flag.Shorthand,
				Usage:     flag.Usage,
				DefValue:  flag.DefValue,
				Changed:   flag.Changed,
				Type:      fmt.Sprintf("%T", flag.Value),
			}
			pbFlags = append(pbFlags, pbFlag)
		})

		pbCmd := &core.Command{
			Use:         cmd.Use,
			Short:       cmd.Short,
			RunFunction: cmd.Run != nil,
			Flags:       pbFlags,
		}

		// Обрабатываем подкоманды
		for _, subCmd := range cmd.Commands() {
			var subPbFlags []*core.Flag
			subCmd.Flags().VisitAll(func(flag *pflag.Flag) {
				subPbFlag := &core.Flag{
					Name:      flag.Name,
					Shorthand: flag.Shorthand,
					Usage:     flag.Usage,
					DefValue:  flag.DefValue,
					Changed:   flag.Changed,
					Type:      fmt.Sprintf("%T", flag.Value),
				}
				subPbFlags = append(subPbFlags, subPbFlag)
			})

			pbCmd.Commands = append(pbCmd.Commands, &core.Command{
				Use:         subCmd.Use,
				Short:       subCmd.Short,
				RunFunction: subCmd.Run != nil,
				Flags:       subPbFlags,
			})
		}

		pbCommands[i] = pbCmd
	}

	return &core.CommandsResponse{Commands: pbCommands}, nil
}

func (s *CommandGRPCServer) RunCommand(ctx context.Context, req *core.CommandRequest) (*core.Empty, error) {
	cmdPath := strings.Split(req.Name, " ")
	commands := s.Impl.GetCommands()

	// Находим нужную команду
	var targetCmd *cobra.Command
	for _, cmd := range commands {
		if cmd.Use == cmdPath[0] {
			targetCmd = cmd
			if len(cmdPath) > 1 {
				for _, subCmd := range cmd.Commands() {
					if subCmd.Use == cmdPath[1] {
						targetCmd = subCmd
						break
					}
				}
			}
			break
		}
	}

	if targetCmd != nil && targetCmd.Run != nil {
		// Устанавливаем флаги
		for name, value := range req.Flags {
			if err := targetCmd.Flags().Set(name, value); err != nil {
				return nil, fmt.Errorf("failed to set flag %s: %v", name, err)
			}
		}

		// Выполняем команду
		targetCmd.Run(targetCmd, req.Args)
	}

	return &core.Empty{}, nil
}

func (s *CommandGRPCServer) Name(ctx context.Context, req *core.Empty) (*core.NameResponse, error) {
	return &core.NameResponse{Name: s.Impl.Name()}, nil
}

func (s *CommandGRPCServer) Version(ctx context.Context, req *core.Empty) (*core.VersionResponse, error) {
	return &core.VersionResponse{Version: s.Impl.Version()}, nil
}

func (p *TestPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	core.RegisterCommandModuleServer(s, &CommandGRPCServer{Impl: p.Impl})
	return nil
}

func (p *TestPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &core.CommandGRPCClient{Client: core.NewCommandModuleClient(c)}, nil
}

func main() {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "test-module",
		Output: os.Stderr,
		Level:  hclog.Info,
	})

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "OCTOCHAN_PLUGIN",
			MagicCookieValue: "octochan-2025",
		},
		Plugins: map[string]plugin.Plugin{
			"command": &TestPlugin{Impl: &TestModule{}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
		Logger:     logger,
	})
}
