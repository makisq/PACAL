// Package module содержит встроенные модули сценариев RLM.
//
// Внешние модули команд подключаются иначе — через go-plugin,
// см. pluginhost.ModuleManager. Прежняя реализация здесь пыталась ещё и
// открывать .so через stdlib plugin: результат никуда не сохранялся,
// ошибки глотались, а на Windows этот механизм не работает вовсе.
package module

import "octochan/rlm"

func init() {
	rlm.RegisterScenarioModule("psqlse_tuningpgbouncer", NewPgBouncerTuningModule)
	rlm.RegisterScenarioModule("psql_tuning_params_se", NewPsqlTuningParamsModule)
	rlm.RegisterScenarioModule("postgresql_se_get_config_files", NewPostgresConfigFilesModule)
	rlm.RegisterScenarioModule("pangolin_restart", NewPangolinRestartModule)
}
