package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"octochan/core"

	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

// logFileName должен совпадать у писателя и у команды logs.
// Раньше журнал писался в rlm-cli.log, а команда logs читала octochan.log,
// поэтому она всегда сообщала, что записей ещё нет.
const logFileName = "octochan.log"

// fileManager — общий на процесс, создаётся один раз в initConfig.
var fileManager *core.FileManager

func mustFileManager() (*core.FileManager, error) {
	if fileManager == nil {
		return nil, fmt.Errorf("рабочие директории не инициализированы")
	}
	return fileManager, nil
}

// configPath определяет, откуда читать конфиг.
// Флаг --config раньше объявлялся, но нигде не использовался.
func configPath(fm *core.FileManager) string {
	if cfgFile != "" {
		return cfgFile
	}
	if env := os.Getenv("OCTOCHAN_CONFIG"); env != "" {
		return env
	}
	return filepath.Join(fm.BaseDir(), "config.yaml")
}

func initConfig() error {
	fm, err := core.NewFileManager()
	if err != nil {
		return err
	}
	fileManager = fm

	// Все пути живут под ~/.octochan. Прежняя версия держала config.yaml
	// в ~/octochan, а всё остальное в ~/.octochan.
	viper.SetDefault("defaults.api_url", "")
	viper.SetDefault("defaults.scenario_path", fm.ScenariosDir())
	viper.SetDefault("defaults.log_path", fm.LogsDir())
	viper.SetDefault("defaults.auto_check_status", false)
	viper.SetDefault("defaults.max_parallel_tasks", 5)
	viper.SetDefault("defaults.log_max_size", 10)
	viper.SetDefault("defaults.log_max_backups", 5)
	viper.SetDefault("defaults.http_timeout", "30s")
	viper.SetDefault("defaults.module_command_timeout", "10m")
	viper.SetDefault("defaults.plugin_log_level", "warn")
	viper.SetDefault("defaults.allow_unverified_modules", false)
	viper.SetDefault("defaults.token_timer_enabled", true)
	viper.SetDefault("defaults.token_lifetime", "20m")

	path := configPath(fm)
	viper.SetConfigFile(path)

	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		// Конфига ещё нет — создаём со значениями по умолчанию.
		if err := viper.WriteConfigAs(path); err != nil {
			return fmt.Errorf("не удалось создать конфиг %s: %w", path, err)
		}
	} else if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("не удалось прочитать конфиг %s: %w", path, err)
	}

	// Токен читается отдельно и в viper не попадает, чтобы не уехать
	// в config.yaml при следующем WriteConfig.
	if err := fm.LoadToken(); err != nil {
		return fmt.Errorf("не удалось прочитать токен: %w", err)
	}

	return nil
}

// initLogger направляет журнал в файл.
//
// Раньше здесь стоял io.MultiWriter(os.Stdout, writer): диагностика уезжала
// в stdout вперемешку с полезным выводом и ломала конвейеры вида
// `ochan print a.conf | ochan diff -`.
func initLogger() error {
	logPath := viper.GetString("defaults.log_path")
	if logPath == "" {
		fm, err := mustFileManager()
		if err != nil {
			return err
		}
		logPath = fm.LogsDir()
	}

	if err := os.MkdirAll(logPath, 0o700); err != nil {
		return fmt.Errorf("не удалось создать директорию логов %s: %w", logPath, err)
	}

	writer := &lumberjack.Logger{
		Filename:   filepath.Join(logPath, logFileName),
		MaxSize:    viper.GetInt("defaults.log_max_size"),
		MaxBackups: viper.GetInt("defaults.log_max_backups"),
		Compress:   true,
	}

	var out io.Writer = writer
	if verbose {
		out = io.MultiWriter(os.Stderr, writer)
	}

	log.SetOutput(out)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	return nil
}
