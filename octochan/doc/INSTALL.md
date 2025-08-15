# Установка OctoChan

Инструкция по установке и настройке языка программирования OctoChan.

## Системные требования

- Linux x86_64
- 8MB свободного места
- Права на выполнение файлов

## Установка

### 1. Скачать OctoChan
```bash
# Скачать последнюю версию
wget https://github.com/octochan/octochan/releases/latest/octochan
chmod +x octochan
```

### 2. Установить в систему
```bash
# Установить глобально
sudo mv octochan /usr/local/bin/
```

### 3. Проверить установку
```bash
octochan eval 'print("OctoChan установлен!")'
```

## Первая программа

### Создать файл hello.octo
```octochan
required_role: "default"

name = "OctoChan"
print("Hello from " + $name + "!")
```

### Запустить программу
```bash
octochan eval 'run_scenario("hello")'
```

## Компиляция в нативный код

```bash
# Компилировать в машинный код
octochan eval 'compile_native("hello")'

# Запустить скомпилированную программу
./hello_bin
```

## Интерактивный режим

```bash
octochan
> print("Hello World!")
> system_info()
> exit
```

## Настройка среды разработки

### VS Code
1. Установить расширение "OctoChan Language Support"
2. Настроить подсветку синтаксиса для .octo файлов

### Vim
```bash
# Добавить в ~/.vimrc
autocmd BufNewFile,BufRead *.octo set filetype=octochan
```

## Обновление

```bash
# Проверить версию
octochan eval 'system_info()'

# Обновить до последней версии
wget https://github.com/octochan/octochan/releases/latest/octochan
chmod +x octochan
sudo mv octochan /usr/local/bin/
```

## Удаление

```bash
sudo rm /usr/local/bin/octochan
rm -rf ~/.octochan
```

---

**Готово! OctoChan установлен и готов к использованию.**