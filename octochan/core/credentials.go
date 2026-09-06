package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// EnvTokenVar — переменная окружения с токеном RLM API.
const EnvTokenVar = "OCTOCHAN_API_TOKEN"

// Токен намеренно живёт вне viper.
//
// Раньше loadEnv() делал viper.Set("defaults.api_token", ...), а команда timer
// вызывала viper.WriteConfig() — и токен, аккуратно сохранённый в .env с
// правами 0600, оказывался в открытом config.yaml с правами 0644.
var (
	tokenMu  sync.RWMutex
	apiToken string
)

// SetAPIToken запоминает токен на время работы процесса.
func SetAPIToken(token string) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	apiToken = strings.TrimSpace(token)
}

// APIToken возвращает токен: сначала из памяти, затем из окружения.
func APIToken() string {
	tokenMu.RLock()
	token := apiToken
	tokenMu.RUnlock()

	if token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv(EnvTokenVar))
}

// TokenFilePath — путь до файла с токеном.
func (fm *FileManager) TokenFilePath() string {
	return filepath.Join(fm.baseDir, ".env")
}

// SaveToken записывает токен в файл с правами 0600.
func (fm *FileManager) SaveToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("пустой токен")
	}

	content := fmt.Sprintf("%s=%s\n", EnvTokenVar, token)
	if err := os.WriteFile(fm.TokenFilePath(), []byte(content), filePerm); err != nil {
		return fmt.Errorf("не удалось сохранить токен: %w", err)
	}

	SetAPIToken(token)
	return nil
}

// LoadToken читает токен из файла и запоминает его.
// Отсутствие файла ошибкой не считается.
func (fm *FileManager) LoadToken() error {
	f, err := os.Open(fm.TokenFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		// Ключ API_TOKEN поддерживается ради файлов, созданных прежней версией.
		switch strings.TrimSpace(key) {
		case EnvTokenVar, "API_TOKEN":
			// TrimSpace снимает в том числе \r из файлов, побывавших в Windows.
			SetAPIToken(strings.Trim(strings.TrimSpace(value), `"'`))
		}
	}

	return scanner.Err()
}
