// Package module содержит встроенные модули сценариев RLM.
//
// Внешние модули команд подключаются иначе — через go-plugin,
// см. core.ModuleManager. Прежняя реализация здесь пыталась ещё и
// открывать .so через stdlib plugin: результат никуда не сохранялся,
// ошибки глотались, а на Windows этот механизм не работает вовсе.
package module

import "octochan/core"

func init() {
	core.RegisterScenarioModule("psqlse_tuningpgbouncer", NewPgBouncerTuningModule)
	core.RegisterScenarioModule("psql_tuning_params_se", NewPsqlTuningParamsModule)
	core.RegisterScenarioModule("postgresql_se_get_config_files", NewPostgresConfigFilesModule)
	core.RegisterScenarioModule("pangolin_restart", NewPangolinRestartModule)
}
