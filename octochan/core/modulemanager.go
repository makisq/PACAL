package core

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

type CommandModule interface {
	GetCommands() []*cobra.Command
	Init(config map[string]interface{}) error
	Name() string
	Version() string
}

type CommandPlugin struct {
	plugin.Plugin
	Impl CommandModule
}

func (p *CommandPlugin) Server(broker *plugin.GRPCBroker, s *grpc.Server) error {
	RegisterCommandModuleServer(s, &CommandGRPCServer{Impl: p.Impl})
	return nil
}

func (p *CommandPlugin) Client(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &CommandGRPCClient{client: NewCommandModuleClient(c)}, nil
}

type CommandGRPCServer struct {
	Impl CommandModule
}

func (s *CommandGRPCServer) GetCommands(ctx context.Context, req *Empty) (*CommandsResponse, error) {
	return &CommandsResponse{Commands: s.Impl.GetCommands()}, nil
}

func (s *CommandGRPCServer) Init(ctx context.Context, config *Config) (*Empty, error) {
	return &Empty{}, s.Impl.Init(config.Values)
}

func (s *CommandGRPCServer) Name(ctx context.Context, req *Empty) (*NameResponse, error) {
	return &NameResponse{Name: s.Impl.Name()}, nil
}

func (s *CommandGRPCServer) Version(ctx context.Context, req *Empty) (*VersionResponse, error) {
	return &VersionResponse{Version: s.Impl.Version()}, nil
}

type CommandGRPCClient struct {
	client CommandModuleClient
}

func (c *CommandGRPCClient) GetCommands() []*cobra.Command {
	resp, err := c.client.GetCommands(context.Background(), &Empty{})
	if err != nil {
		return nil
	}
	return resp.Commands
}

func (c *CommandGRPCClient) Init(config map[string]interface{}) error {
	_, err := c.client.Init(context.Background(), &Config{Values: config})
	return err
}

func (c *CommandGRPCClient) Name() string {
	resp, err := c.client.Name(context.Background(), &Empty{})
	if err != nil {
		return ""
	}
	return resp.Name
}

func (c *CommandGRPCClient) Version() string {
	resp, err := c.client.Version(context.Background(), &Empty{})
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

	// Подключаемся через GRPC
	grpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return fmt.Errorf("GRPC connection failed: %w", err)
	}

	// Получаем интерфейс плагина
	raw, err := grpcClient.Dispense("command")
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
	m.commands = append(m.commands, module.GetCommands()...)

	logger.Info("Plugin loaded successfully",
		"name", module.Name(),
		"version", module.Version())

	return nil
}

func (m *ModuleManager) GetCommands() []*cobra.Command {
	m.mu.Lock()
	defer m.mu.Unlock()
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
