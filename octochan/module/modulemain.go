package module

import (
	"octochan/core"
	"os"
	"path/filepath"
	"plugin"
	"sync"
)

var (
	modulesLoaded   bool
	modulesLoadLock sync.Mutex
)

func AutoRegisterModules() {
	modulesLoadLock.Lock()
	defer modulesLoadLock.Unlock()

	if modulesLoaded {
		return
	}

	fm, err := core.NewFileManager()
	if err != nil {
		return
	}

	loadModulesFromDir(fm.ModulesDir())

	modulesLoaded = true
}

func loadModulesFromDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".so" {
			modulePath := filepath.Join(dir, entry.Name())
			_, err := plugin.Open(modulePath)
			if err != nil {
				continue
			}
		}
	}
}



func init() {
	core.RegisterScenarioModule("psqlse_tuningpgbouncer", NewPgBouncerTuningModule)
	core.RegisterScenarioModule("psql_tuning_params_se", NewPsqlTuningParamsModule)
	core.RegisterScenarioModule("postgresql_se_get_config_files", NewPostgresConfigFilesModule)
	core.RegisterScenarioModule("pangolin_restart", NewPangolinRestartModule)
	AutoRegisterModules()
}
