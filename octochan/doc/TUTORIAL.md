# OctoChan Tutorial

Пошаговое изучение языка программирования OctoChan.

## Урок 1: Основы

### Переменные
```octochan
# Создание переменных
name = "Alice"
age = 25
active = true

# Использование переменных
greeting = "Hello, " + $name
print($greeting)
```

### Функции
```octochan
# Вызов функций
info = system_info()
user = user_info()
print($info)
```

## Урок 2: Конвейеры

### Простые конвейеры
```octochan
# Обработка данных через конвейер
result = system_info() |> filter("version")
print($result)
```

### Сложные конвейеры
```octochan
# Многоэтапная обработка
data = load_data() |>
       validate() |>
       transform() |>
       save()
```

## Урок 3: Алиасы

### Создание алиасов
```octochan
# Простые алиасы
create_alias("info", "system_info")
create_alias("user", "user_info")

# Использование
info()  # → system_info()
user()  # → user_info()
```

### Многоуровневые алиасы
```octochan
create_alias("get_info", "info")
create_alias("full_info", "get_info")

full_info()  # → get_info → info → system_info
```

## Урок 4: Условия и логика

### Простые условия
```octochan
status = "success"
if $status == "success" {
    print("Операция выполнена успешно!")
}
```

### Сложные условия
```octochan
user_role = "admin"
environment = "production"

if $user_role == "admin" && $environment == "production" {
    print("Доступ к продакшену разрешен")
} else {
    print("Доступ запрещен")
}
```

## Урок 5: Объекты и структуры

### Создание объектов
```octochan
config = {
    "host": "localhost",
    "port": 8080,
    "ssl": true
}

# Доступ к полям
host = $config.host
port = $config.port
```

### Вложенные объекты
```octochan
server = {
    "name": "web-server",
    "config": {
        "host": "0.0.0.0",
        "port": 80
    },
    "status": "running"
}

server_host = $server.config.host
```

## Урок 6: Массивы

### Работа с массивами
```octochan
servers = ["web1", "web2", "web3"]
ports = [80, 443, 8080]

# Обработка массивов
active_servers = $servers |> filter("active")
server_count = $servers |> count()
```

## Урок 7: Сценарии

### Создание сценария (deploy.octo)
```octochan
required_role: "admin"

# Переменные сценария
environment = "production"
app_name = "web-service"

# Логика развертывания
if has_permission("deploy") {
    result = deploy_app($app_name, $environment)
    
    if $result.status == "success" {
        print("Развертывание успешно!")
    } else {
        print("Ошибка развертывания: " + $result.error)
    }
}
```

### Запуск сценария
```octochan
run_scenario("deploy")
```

## Урок 8: Система безопасности (RBAC)

### Создание ролей
```octochan
# Создать роль с правами
create_role("developer", "read,write,test")
create_role("admin", "read,write,deploy,manage")

# Назначить роль пользователю
assign_role("alice", "developer")
assign_role("bob", "admin")
```

### Проверка прав
```octochan
# Проверить права текущего пользователя
perms = check_permissions()
print($perms)

# Проверить конкретное право
if has_permission("deploy") {
    print("Можно развертывать")
} else {
    print("Нет прав на развертывание")
}
```

## Урок 9: Компиляция

### Компиляция в нативный код
```octochan
# Скомпилировать программу
compile_native("my_program")

# Результат: my_program_bin (исполняемый файл)
```

### Самокомпиляция
```octochan
# OctoChan компилирует сам себя!
bootstrap_result = run_scenario("bootstrap_compiler")
print($bootstrap_result)
```

## Урок 10: Практические примеры

### Веб-сервер
```octochan
# web_server.octo
required_role: "admin"

server_config = {
    "host": "0.0.0.0",
    "port": 8080,
    "routes": ["/", "/api", "/health"]
}

start_server($server_config)
print("Сервер запущен на порту " + $server_config.port)
```

### Мониторинг системы
```octochan
# monitor.octo
required_role: "ops"

# Получить метрики
cpu = system_info() |> filter("cpu")
memory = system_info() |> filter("memory")

# Проверить состояние
if $cpu > 80 {
    alert("Высокая загрузка CPU: " + $cpu + "%")
}

if $memory > 90 {
    alert("Мало памяти: " + $memory + "%")
}
```

### CI/CD Pipeline
```octochan
# pipeline.octo
required_role: "ci"

# Этапы пайплайна
create_alias("test", "run_tests")
create_alias("build", "compile_native")
create_alias("deploy", "deploy_app")

# Выполнение пайплайна
app_name = "my-service"
result = $app_name |>
         test() |>
         build() |>
         deploy("production")

if $result.status == "success" {
    notify("team", "Развертывание успешно!")
} else {
    notify("team", "Ошибка в пайплайне: " + $result.error)
}
```

## Заключение

Поздравляем! Вы изучили основы OctoChan:

✅ Переменные и функции
✅ Конвейеры для обработки данных
✅ Алиасы для переиспользования кода
✅ Условия и логику
✅ Объекты и массивы
✅ Сценарии и модульность
✅ Систему безопасности (RBAC)
✅ Компиляцию в нативный код
✅ Практические примеры

**Теперь вы готовы создавать мощные программы на OctoChan!**

---

Следующий шаг: изучите полную документацию в `OCTOCHAN_LANGUAGE_REFERENCE.txt`