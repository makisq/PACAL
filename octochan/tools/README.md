# OctoChan Editor Support

Подсветка синтаксиса и поддержка языка OctoChan для популярных редакторов.

## Visual Studio Code

### Установка расширения
1. Скопировать папку `vscode-extension` в `~/.vscode/extensions/`
2. Перезапустить VS Code
3. Открыть файл `.octo` - подсветка включится автоматически

### Возможности
- ✅ Подсветка синтаксиса
- ✅ Автозакрытие скобок
- ✅ Комментарии (#)
- ✅ Автоотступы

## Vim/Neovim

### Установка
```bash
# Скопировать файл синтаксиса
cp tools/vim-syntax/octochan.vim ~/.vim/syntax/

# Добавить в ~/.vimrc
echo "autocmd BufNewFile,BufRead *.octo set filetype=octochan" >> ~/.vimrc
```

### Возможности
- ✅ Подсветка ключевых слов
- ✅ Подсветка функций
- ✅ Подсветка переменных ($var)
- ✅ Подсветка операторов (|>)

## Sublime Text

### Установка
1. Открыть Sublime Text
2. Tools → Developer → New Syntax
3. Заменить содержимое на `tools/sublime-syntax/OctoChan.sublime-syntax`
4. Сохранить как `OctoChan.sublime-syntax`

### Возможности
- ✅ Подсветка синтаксиса
- ✅ Автодополнение
- ✅ Сворачивание блоков

## Поддерживаемые элементы

### Ключевые слова
```octochan
if else for while in required_role
create_alias run_scenario list_scenarios has_permission
```

### Функции
```octochan
print system_info user_info apply_diff filter count compile_native
```

### Переменные
```octochan
$variable_name
variable = "value"
```

### Операторы
```octochan
|>  # Pipeline operator
== != <= >= < > + - * / =
```

### Строки и комментарии
```octochan
"string value"
# comment
```

### Роли
```octochan
required_role: "admin"
```

## Пример с подсветкой

```octochan
# OctoChan program with syntax highlighting
required_role: "default"

# Variables
name = "OctoChan"
version = 3

# Pipeline with functions
result = system_info() |> 
         filter("version") |> 
         print()

# Conditional logic
if $version > 2 {
    print("Modern OctoChan!")
}

# Aliases
create_alias("info", "system_info")
info()
```

## Цветовая схема

- **Ключевые слова**: синий
- **Функции**: зеленый  
- **Строки**: красный
- **Комментарии**: серый
- **Переменные**: фиолетовый
- **Операторы**: оранжевый
- **Числа**: голубой

---

**Теперь OctoChan выглядит красиво в любом редакторе!**