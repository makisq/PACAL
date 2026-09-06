# octochan

Утилита сравнения конфигураций PostgreSQL/pgbouncer и запуска сценариев RLM.

## Сборка

```
go build -o ochan .
```

Собирается под Linux, macOS и Windows.

## Рабочие директории

Всё живёт под `~/.octochan`:

```
~/.octochan/
  config.yaml     настройки
  .env            токен API (права 0600)
  modules/        подключаемые модули команд (*.hcplugin) и их суммы
  scenarios/      сохранённые сценарии
  reports/        отчёты команды diff
  snapshots/      снапшоты расхождений
  logs/           журнал работы
```

Путь до конфига переопределяется флагом `--config` или переменной
`OCTOCHAN_CONFIG`.

## Настройка

Адрес API не зашит в код — задайте его перед первым запуском:

```yaml
# ~/.octochan/config.yaml
defaults:
  api_url: https://rlm.example.internal/api/tasks.json
  max_parallel_tasks: 5
  http_timeout: 30s
  module_command_timeout: 10m
  auto_check_status: false
  allow_unverified_modules: false
```

Токен:

```
ochan auth                       # спросит токен, ввод скрыт
export OCTOCHAN_API_TOKEN=...    # либо через окружение
```

Токен хранится в `~/.octochan/.env` с правами `0600` и никогда не попадает
в `config.yaml`.

## Основные команды

```
ochan diff prod.conf test.conf        сравнить два конфига
ochan patch base.conf changes.conf -o out.conf
ochan validate postgresql.conf        проверить синтаксис
ochan find postgresql.conf shared_buffers
ochan stats postgresql.conf
ochan apply --rlm scenario.yaml       выполнить сценарий RLM
ochan apply --rlm scenario.yaml --wait
ochan status <task_id>
ochan logs --tail 50
ochan shell                           интерактивная оболочка
```

Команды читают stdin, если вместо файла указан `-` или файл не указан вовсе:

```
cat prod.conf | ochan validate
ochan print prod.conf | ochan diff - test.conf
```

Ненулевой код возврата означает ошибку — команды пригодны для CI.

## Сравнение значений

Значения сравниваются по смыслу, а не побуквенно:

| Равны | Не равны |
|---|---|
| `8GB` и `8192MB` | `1` и `on` |
| `1s` и `1000ms` | `8MB` и `8GB` |
| `on`, `true`, `yes` | `/var/lib/PG` и `/var/lib/pg` |

## Модули команд

Модуль — отдельный исполняемый файл, реализующий `core.CommandModule`
(см. `dwd/main.go`). Обвязка go-plugin и gRPC живёт в `core`.

```
go build -o test-module.hcplugin ./dwd
ochan '!install' ./test-module.hcplugin
ochan modules
```

При установке фиксируется SHA-256 модуля. При запуске загружаются только
модули с совпадающей суммой — файл, подменённый в `~/.octochan/modules`
после установки, выполнен не будет. Для разработки проверку можно отключить
через `defaults.allow_unverified_modules: true`.

В интерактивной оболочке изменившиеся модули перезагружаются автоматически;
после пересборки модуль нужно переустановить (`!install`), чтобы обновить
контрольную сумму.

## Тесты

```
go test ./...
go test -race ./...
go vet ./...
```
