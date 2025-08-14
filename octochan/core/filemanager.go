package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

func ExecuteScenarioFromFile(path string, customParams map[string]string) ([]string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла: %w", err)
	}
	return ExecuteModularScenario(data, customParams)
}
func GetScenariosDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("не удалось определить домашнюю директорию: %w", err)
	}

	dir := filepath.Join(home, ".octochan", "scenarios")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("не удалось создать директорию сценариев: %w", err)
	}

	return dir, nil
}

type FileManager struct {
	baseDir      string
	scenariosDir string
	modulesDir   string
	logsDir      string
}

func NewFileManager() (*FileManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("не удалось определить домашнюю директорию: %w", err)
	}

	base := filepath.Join(home, ".octochan")
	dirs := map[string]string{
		"scenarios": filepath.Join(base, "scenarios"),
		"modules":   filepath.Join(base, "modules"),
		"logs":      filepath.Join(base, "logs"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("не удалось создать директорию %s: %w", dir, err)
		}
	}

	return &FileManager{
		baseDir:      base,
		scenariosDir: dirs["scenarios"],
		modulesDir:   dirs["modules"],
		logsDir:      dirs["logs"],
	}, nil
}

func (fm *FileManager) BaseDir() string {
	return fm.baseDir
}

func (fm *FileManager) ScenariosDir() string {
	return fm.scenariosDir
}

func (fm *FileManager) ModulesDir() string {
	return fm.modulesDir
}

func (fm *FileManager) LogsDir() string {
	return fm.logsDir
}

func (fm *FileManager) InstallModule(name string, data []byte) error {
	modulePath := filepath.Join(fm.modulesDir, name)
	return os.WriteFile(modulePath, data, 0644)
}

func (fm *FileManager) SaveScenario(name string, data []byte) error {
	path := filepath.Join(fm.scenariosDir, name+".bin")
	return os.WriteFile(path, data, 0644)
}

func (fm *FileManager) LoadScenario(name string) ([]byte, error) {
	path := filepath.Join(fm.BaseDir(), name+".bin")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("файл сценария не существует")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("файл сценария пуст")
	}

	return data, nil
}

func (fm *FileManager) InstallHashicorpModule(name string, data []byte) error {

	modulePath := filepath.Join(fm.modulesDir, name)

	if err := os.WriteFile(modulePath, data, 0755); err != nil {
		return err
	}

	return nil
}
