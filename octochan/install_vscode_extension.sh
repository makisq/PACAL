#!/bin/bash

echo "=== Установка OctoChan расширения для VS Code ==="
echo ""

# Определяем папку расширений VS Code
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    EXTENSIONS_DIR="$HOME/.vscode/extensions"
elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" ]]; then
    # Windows
    EXTENSIONS_DIR="$USERPROFILE/.vscode/extensions"
else
    # Linux
    EXTENSIONS_DIR="$HOME/.vscode/extensions"
fi

echo "Папка расширений VS Code: $EXTENSIONS_DIR"

# Проверяем существование папки
if [ ! -d "$EXTENSIONS_DIR" ]; then
    echo "❌ Папка расширений VS Code не найдена!"
    echo "Убедитесь, что VS Code установлен."
    exit 1
fi

# Создаем папку для расширения
EXTENSION_NAME="octochan-language-support-1.0.0"
TARGET_DIR="$EXTENSIONS_DIR/$EXTENSION_NAME"

echo "Создаем папку расширения: $TARGET_DIR"
mkdir -p "$TARGET_DIR"

# Копируем файлы расширения
echo "Копируем файлы расширения..."
cp -r tools/vscode-extension/* "$TARGET_DIR/"

# Проверяем установку
if [ -f "$TARGET_DIR/package.json" ]; then
    echo "✅ Расширение успешно установлено!"
    echo ""
    echo "Следующие шаги:"
    echo "1. Перезапустите VS Code"
    echo "2. Откройте файл .octo"
    echo "3. Наслаждайтесь подсветкой синтаксиса!"
    echo ""
    echo "Тестовый файл:"
    echo "code examples/syntax_showcase.octo"
else
    echo "❌ Ошибка установки расширения"
    exit 1
fi

# Создаем тестовый файл
echo "Создаем тестовый файл..."
cat > test_octochan.octo << 'EOF'
required_role: "default"

# OctoChan test file with syntax highlighting
name = "OctoChan"
version = 3

# Function call
info = system_info()

# Pipeline
result = $info |> filter("version") |> print()

# Conditional
if $version >= 3 {
    print("Modern OctoChan!")
}

# Alias
create_alias("hello", "print")
hello("Hello from VS Code!")
EOF

echo "✅ Тестовый файл создан: test_octochan.octo"
echo ""
echo "🎉 Установка завершена!"
echo "Откройте test_octochan.octo в VS Code чтобы увидеть подсветку!"