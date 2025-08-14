package core

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"

	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/grpc"
)

type CommandModule interface {
	GetCommands() []*cobra.Command
	Init(config map[string]interface{}) error
	Name() string
	Version() string
}

type CommandPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	Impl CommandModule
}

func (p *CommandPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	RegisterCommandModuleServer(s, &CommandGRPCServer{Impl: p.Impl})
	return nil
}

func (p *CommandPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &CommandGRPCClient{Client: NewCommandModuleClient(c)}, nil
}

func (p *CommandPlugin) Server(broker *plugin.MuxBroker) (interface{}, error) {
	return nil, nil
}

type CommandGRPCServer struct {
	UnimplementedCommandModuleServer
	Impl CommandModule
}

func (s *CommandGRPCServer) GetCommands(ctx context.Context, req *Empty) (*CommandsResponse, error) {
	commands := s.Impl.GetCommands()
	pbCommands := make([]*Command, len(commands))

	for i, cmd := range commands {
		var pbFlags []*Flag
		if cmd.Flags() != nil {
			cmd.Flags().VisitAll(func(flag *pflag.Flag) {
				pbFlag := &Flag{
					Name:      flag.Name,
					Shorthand: flag.Shorthand,
					Usage:     flag.Usage,
					DefValue:  flag.DefValue,
					Changed:   flag.Changed,
					Type:      fmt.Sprintf("%T", flag.Value),
				}
				pbFlags = append(pbFlags, pbFlag)
			})
		}

		pbAnnotations := make(map[string]string)
		if cmd.Annotations != nil {
			for k, v := range cmd.Annotations {
				pbAnnotations[k] = v
			}
		}

		pbCommands[i] = &Command{
			Use:                       cmd.Use,
			Short:                     cmd.Short,
			Long:                      cmd.Long,
			Example:                   cmd.Example,
			ValidArgs:                 cmd.ValidArgs,
			ValidArgsFunction:         cmd.ValidArgsFunction != nil,
			Args:                      cmd.Args != nil,
			ArgAliases:                cmd.ArgAliases,
			BashCompletionFunction:    cmd.BashCompletionFunction,
			Deprecated:                cmd.Deprecated,
			Hidden:                    cmd.Hidden,
			Annotations:               pbAnnotations,
			Version:                   cmd.Version,
			PersistentFlagSet:         cmd.HasPersistentFlags(),
			LocalFlagSet:              cmd.HasLocalFlags(),
			Flags:                     pbFlags,
			RunFunction:               cmd.Run != nil, // Только маркер
			PreRunFunction:            cmd.PreRun != nil,
			PostRunFunction:           cmd.PostRun != nil,
			PersistentPreRunFunction:  cmd.PersistentPreRun != nil,
			PersistentPostRunFunction: cmd.PersistentPostRun != nil,
		}
	}

	return &CommandsResponse{Commands: pbCommands}, nil
}

func (s *CommandGRPCServer) Name(ctx context.Context, req *Empty) (*NameResponse, error) {
	return &NameResponse{Name: s.Impl.Name()}, nil
}

func (s *CommandGRPCServer) Version(ctx context.Context, req *Empty) (*VersionResponse, error) {
	return &VersionResponse{Version: s.Impl.Version()}, nil
}

type CommandGRPCClient struct {
	Client CommandModuleClient
}

func (c *CommandGRPCClient) GetCommands() []*cobra.Command {
	resp, err := c.Client.GetCommands(context.Background(), &Empty{})
	if err != nil {
		log.Printf("Failed to get commands from plugin: %v", err)
		return nil
	}
	return c.CreateCommandsFromProto(resp.Commands)
}

func (c *CommandGRPCClient) CreateCommandsFromProto(pbCommands []*Command) []*cobra.Command {
	commands := make([]*cobra.Command, len(pbCommands))

	for i, pbCmd := range pbCommands {
		cmd := &cobra.Command{
			Use:                    pbCmd.Use,
			Aliases:                pbCmd.Aliases,
			Short:                  pbCmd.Short,
			Long:                   pbCmd.Long,
			Example:                pbCmd.Example,
			ValidArgs:              pbCmd.ValidArgs,
			ArgAliases:             pbCmd.ArgAliases,
			BashCompletionFunction: pbCmd.BashCompletionFunction,
			Deprecated:             pbCmd.Deprecated,
			Hidden:                 pbCmd.Hidden,
			Annotations:            pbCmd.Annotations,
			Version:                pbCmd.Version,
			SilenceErrors:          pbCmd.SilenceErrors,
			SilenceUsage:           pbCmd.SilenceUsage,
			TraverseChildren:       pbCmd.TraverseChildren,
			FParseErrWhitelist: cobra.FParseErrWhitelist{
				UnknownFlags: pbCmd.FparseErrWhitelist.GetUnknownFlags(),
			},
		}

		if pbCmd.RunFunction {
			cmd.Run = func(cmd *cobra.Command, args []string) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_, err := c.Client.RunCommand(ctx, &CommandRequest{
					Name: cmd.Use,
					Args: args,
				})
				if err != nil {
					cmd.PrintErrln("Command execution error:", err)
				}
			}
		}

		if pbCmd.PreRunFunction {
			cmd.PreRun = func(cmd *cobra.Command, args []string) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := c.Client.PreRunCommand(ctx, &CommandRequest{
					Name: cmd.Use,
					Args: args,
				})
				if err != nil {
					cmd.PrintErrln("PreRun error:", err)
				}
			}
		}

		if pbCmd.PostRunFunction {
			cmd.PostRun = func(cmd *cobra.Command, args []string) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := c.Client.PostRunCommand(ctx, &CommandRequest{
					Name: cmd.Use,
					Args: args,
				})
				if err != nil {
					cmd.PrintErrln("PostRun error:", err)
				}
			}
		}

		for _, pbFlag := range pbCmd.Flags {
			switch {
			case strings.Contains(pbFlag.Type, "bool"):
				val, err := strconv.ParseBool(pbFlag.DefValue)
				if err != nil {
					log.Printf("Invalid bool value for flag %s: %v", pbFlag.Name, err)
					val = false
				}
				cmd.Flags().BoolP(pbFlag.Name, pbFlag.Shorthand, val, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "int"):
				intVal, err := strconv.Atoi(pbFlag.DefValue)
				if err != nil {
					log.Printf("Invalid int value for flag %s: %v", pbFlag.Name, err)
					intVal = 0
				}
				cmd.Flags().IntP(pbFlag.Name, pbFlag.Shorthand, intVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "int32"):
				intVal, err := strconv.ParseInt(pbFlag.DefValue, 10, 32)
				if err != nil {
					log.Printf("Invalid int32 value for flag %s: %v", pbFlag.Name, err)
					intVal = 0
				}
				cmd.Flags().Int32P(pbFlag.Name, pbFlag.Shorthand, int32(intVal), pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "int64"):
				intVal, err := strconv.ParseInt(pbFlag.DefValue, 10, 64)
				if err != nil {
					log.Printf("Invalid int64 value for flag %s: %v", pbFlag.Name, err)
					intVal = 0
				}
				cmd.Flags().Int64P(pbFlag.Name, pbFlag.Shorthand, intVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "float32"):
				floatVal, err := strconv.ParseFloat(pbFlag.DefValue, 32)
				if err != nil {
					log.Printf("Invalid float32 value for flag %s: %v", pbFlag.Name, err)
					floatVal = 0
				}
				cmd.Flags().Float32P(pbFlag.Name, pbFlag.Shorthand, float32(floatVal), pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "float64"):
				floatVal, err := strconv.ParseFloat(pbFlag.DefValue, 64)
				if err != nil {
					log.Printf("Invalid float64 value for flag %s: %v", pbFlag.Name, err)
					floatVal = 0
				}
				cmd.Flags().Float64P(pbFlag.Name, pbFlag.Shorthand, floatVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "stringSlice"):
				var sliceVal []string
				if pbFlag.DefValue != "" {
					sliceVal = strings.Split(pbFlag.DefValue, ",")
					for i, v := range sliceVal {
						sliceVal[i] = strings.TrimSpace(v)
					}
				}
				cmd.Flags().StringSliceP(pbFlag.Name, pbFlag.Shorthand, sliceVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "stringArray"):
				var arrVal []string
				if pbFlag.DefValue != "" {
					arrVal = strings.Split(pbFlag.DefValue, ",")
				}
				cmd.Flags().StringArrayP(pbFlag.Name, pbFlag.Shorthand, arrVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "duration"):
				durVal, err := time.ParseDuration(pbFlag.DefValue)
				if err != nil {
					log.Printf("Invalid duration value for flag %s: %v", pbFlag.Name, err)
					durVal = 0
				}
				cmd.Flags().DurationP(pbFlag.Name, pbFlag.Shorthand, durVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "time"):
				cmd.Flags().StringP(pbFlag.Name, pbFlag.Shorthand, pbFlag.DefValue, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "bytes"):
				bytesVal, err := base64.StdEncoding.DecodeString(pbFlag.DefValue)
				if err != nil {
					log.Printf("Invalid bytes value for flag %s: %v", pbFlag.Name, err)
					bytesVal = []byte{}
				}
				cmd.Flags().BytesHexP(pbFlag.Name, pbFlag.Shorthand, bytesVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "count"):
				cmd.Flags().CountP(pbFlag.Name, pbFlag.Shorthand, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "string"):
				cmd.Flags().StringP(pbFlag.Name, pbFlag.Shorthand, pbFlag.DefValue, pbFlag.Usage)

			default:
				log.Printf("Unsupported flag type: %s for flag %s", pbFlag.Type, pbFlag.Name)
				cmd.Flags().StringP(pbFlag.Name, pbFlag.Shorthand, pbFlag.DefValue, pbFlag.Usage)
			}
		}

		for _, pbFlag := range pbCmd.PersistentFlags {
			switch {
			case strings.Contains(pbFlag.Type, "bool"):
				val, _ := strconv.ParseBool(pbFlag.DefValue)
				cmd.PersistentFlags().BoolP(pbFlag.Name, pbFlag.Shorthand, val, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "int"):
				intVal, _ := strconv.Atoi(pbFlag.DefValue)
				cmd.PersistentFlags().IntP(pbFlag.Name, pbFlag.Shorthand, intVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "int32"):
				intVal, _ := strconv.ParseInt(pbFlag.DefValue, 10, 32)
				cmd.PersistentFlags().Int32P(pbFlag.Name, pbFlag.Shorthand, int32(intVal), pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "int64"):
				intVal, _ := strconv.ParseInt(pbFlag.DefValue, 10, 64)
				cmd.PersistentFlags().Int64P(pbFlag.Name, pbFlag.Shorthand, intVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "float32"):
				floatVal, _ := strconv.ParseFloat(pbFlag.DefValue, 32)
				cmd.PersistentFlags().Float32P(pbFlag.Name, pbFlag.Shorthand, float32(floatVal), pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "float64"):
				floatVal, _ := strconv.ParseFloat(pbFlag.DefValue, 64)
				cmd.PersistentFlags().Float64P(pbFlag.Name, pbFlag.Shorthand, floatVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "stringSlice"):
				var sliceVal []string
				if pbFlag.DefValue != "" {
					sliceVal = strings.Split(pbFlag.DefValue, ",")
				}
				cmd.PersistentFlags().StringSliceP(pbFlag.Name, pbFlag.Shorthand, sliceVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "stringArray"):
				var arrVal []string
				if pbFlag.DefValue != "" {
					arrVal = strings.Split(pbFlag.DefValue, ",")
				}
				cmd.PersistentFlags().StringArrayP(pbFlag.Name, pbFlag.Shorthand, arrVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "duration"):
				durVal, _ := time.ParseDuration(pbFlag.DefValue)
				cmd.PersistentFlags().DurationP(pbFlag.Name, pbFlag.Shorthand, durVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "time"):
				cmd.PersistentFlags().StringP(pbFlag.Name, pbFlag.Shorthand, pbFlag.DefValue, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "bytes"):
				bytesVal, _ := base64.StdEncoding.DecodeString(pbFlag.DefValue)
				cmd.PersistentFlags().BytesHexP(pbFlag.Name, pbFlag.Shorthand, bytesVal, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "count"):
				cmd.PersistentFlags().CountP(pbFlag.Name, pbFlag.Shorthand, pbFlag.Usage)

			case strings.Contains(pbFlag.Type, "string"):
				cmd.PersistentFlags().StringP(pbFlag.Name, pbFlag.Shorthand, pbFlag.DefValue, pbFlag.Usage)

			default:
				log.Printf("Unsupported persistent flag type: %s for flag %s", pbFlag.Type, pbFlag.Name)
				cmd.PersistentFlags().StringP(pbFlag.Name, pbFlag.Shorthand, pbFlag.DefValue, pbFlag.Usage)
			}
		}

		if len(pbCmd.Commands) > 0 {
			subCommands := c.CreateCommandsFromProto(pbCmd.Commands)
			for _, subCmd := range subCommands {
				cmd.AddCommand(subCmd)
			}
		}

		commands[i] = cmd
	}

	return commands
}

func (c *CommandGRPCClient) Init(config map[string]interface{}) error {
	values := make(map[string]string)
	for k, v := range config {
		if s, ok := v.(string); ok {
			values[k] = s
		}
	}
	_, err := c.Client.Init(context.Background(), &Config{Values: values})
	return err
}

func (c *CommandGRPCClient) Name() string {
	resp, err := c.Client.Name(context.Background(), &Empty{})
	if err != nil {
		return ""
	}
	return resp.Name
}

func (c *CommandGRPCClient) Version() string {
	resp, err := c.Client.Version(context.Background(), &Empty{})
	if err != nil {
		return ""
	}
	return resp.Version
}

type ModuleManager struct {
	commands      []*cobra.Command
	pluginClients map[string]*plugin.Client
	mu            sync.Mutex
}

var (
	globalModuleManager *ModuleManager
	once                sync.Once
)

func init() {
	once.Do(func() {
		globalModuleManager = &ModuleManager{
			pluginClients: make(map[string]*plugin.Client),
		}
	})
}

func GetModuleManager() *ModuleManager {
	return globalModuleManager
}

func (m *ModuleManager) LoadHashicorpPlugin(path string) error {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin-loader",
		Output: os.Stdout,
		Level:  hclog.Debug,
	})

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "OCTOCHAN_PLUGIN",
			MagicCookieValue: "octochan-2025",
		},
		Plugins: map[string]plugin.Plugin{
			"command": &CommandPlugin{},
		},
		Cmd: exec.Command(path),
		AllowedProtocols: []plugin.Protocol{
			plugin.ProtocolGRPC,
		},
		Logger:       logger,
		StartTimeout: 30 * time.Second,
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("GRPC connection failed: %w", err)
	}

	raw, err := rpcClient.Dispense("command")
	if err != nil {
		client.Kill()
		return fmt.Errorf("failed to dispense command interface: %w", err)
	}

	module, ok := raw.(CommandModule)
	if !ok {
		client.Kill()
		return fmt.Errorf("invalid module type: expected CommandModule")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.pluginClients[module.Name()] = client

	if grpcClient, ok := module.(*CommandGRPCClient); ok {
		cobraCommands := module.GetCommands()
		pbCommands := make([]*Command, len(cobraCommands))
		for i, cmd := range cobraCommands {
			pbCommands[i] = convertCobraToProto(cmd)
		}
		m.commands = append(m.commands, grpcClient.CreateCommandsFromProto(pbCommands)...)
	} else {
		m.commands = append(m.commands, module.GetCommands()...)
	}

	logger.Info("Plugin loaded successfully",
		"name", module.Name(),
		"version", module.Version())

	return nil
}

func convertCobraToProto(cmd *cobra.Command) *Command {
	var pbFlags []*Flag
	if cmd.Flags() != nil {
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			pbFlag := &Flag{
				Name:      flag.Name,
				Shorthand: flag.Shorthand,
				Usage:     flag.Usage,
				DefValue:  flag.DefValue,
				Changed:   flag.Changed,
				Type:      fmt.Sprintf("%T", flag.Value),
			}
			pbFlags = append(pbFlags, pbFlag)
		})
	}

	pbAnnotations := make(map[string]string)
	if cmd.Annotations != nil {
		for k, v := range cmd.Annotations {
			pbAnnotations[k] = v
		}
	}

	return &Command{
		Use:                       cmd.Use,
		Short:                     cmd.Short,
		Long:                      cmd.Long,
		Example:                   cmd.Example,
		ValidArgs:                 cmd.ValidArgs,
		ValidArgsFunction:         cmd.ValidArgsFunction != nil,
		Args:                      cmd.Args != nil,
		ArgAliases:                cmd.ArgAliases,
		BashCompletionFunction:    cmd.BashCompletionFunction,
		Deprecated:                cmd.Deprecated,
		Hidden:                    cmd.Hidden,
		Annotations:               pbAnnotations,
		Version:                   cmd.Version,
		PersistentFlagSet:         cmd.HasPersistentFlags(),
		LocalFlagSet:              cmd.HasLocalFlags(),
		Flags:                     pbFlags,
		RunFunction:               cmd.Run != nil,
		PreRunFunction:            cmd.PreRun != nil,
		PostRunFunction:           cmd.PostRun != nil,
		PersistentPreRunFunction:  cmd.PersistentPreRun != nil,
		PersistentPostRunFunction: cmd.PersistentPostRun != nil,
	}
}

func (m *ModuleManager) GetCommands() []*cobra.Command {
	m.mu.Lock()
	defer m.mu.Unlock()
	commands := make([]*cobra.Command, len(m.commands))
	copy(commands, m.commands)
	return m.commands
}

func (m *ModuleManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.pluginClients {
		client.Kill()
		delete(m.pluginClients, name)
	}
}

func (m *ModuleManager) LoadModulesFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read modules directory: %w", err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".hcplugin" {
			modulePath := filepath.Join(dir, entry.Name())
			if err := m.LoadHashicorpPlugin(modulePath); err != nil {
				log.Printf("Failed to load plugin %s: %v", entry.Name(), err)
				continue
			}
		}
	}
	return nil
}
