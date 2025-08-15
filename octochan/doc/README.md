# OctoChan Programming Language

Самохостящийся язык программирования для автоматизации и системного администрирования.

## Особенности

- 🚀 **Самохостинг** - компилятор написан на OctoChan
- ⚡ **Нативная компиляция** - машинный код без зависимостей
- 🔒 **Встроенная безопасность** - система ролей и прав (RBAC)
- 🔄 **Конвейеры** - элегантная обработка данных
- 📦 **Минимальный размер** - бинарники от 8KB
- 🎯 **Простой синтаксис** - легко изучать и использовать

## Быстрый старт

### Hello World
```octochan
# hello.octo
required_role: "default"

print("Hello, OctoChan World!")
```

### Конвейеры
```octochan
result = system_info() |> filter("version") |> print()
```

### Алиасы
```octochan
create_alias("info", "system_info")
info()  # → system_info()
```

## Структура проекта

```
octochan/
├── octochan_main.octo          # Главный файл языка
├── octochan_core.octo          # Ядро языка
├── octochan_stdlib.octo        # Стандартная библиотека
├── scenarios/                  # Компилятор и утилиты
│   ├── bootstrap_compiler.octo # Самокомпиляция
│   ├── lexer.octo             # Лексический анализатор
│   ├── parser.octo            # Синтаксический анализатор
│   ├── codegen.octo           # Генератор кода
│   └── assembler.octo         # Ассемблер
├── examples/                   # Примеры программ
└── OCTOCHAN_LANGUAGE_REFERENCE.txt # Полная документация
```

## Документация

Полное описание языка находится в файле `OCTOCHAN_LANGUAGE_REFERENCE.txt`

## Лицензия

MIT License - свободное использование и модификация.

---

**OctoChan v3.0** - Язык программирования будущего!