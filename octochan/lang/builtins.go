package lang

import (
	"fmt"
	"os"
	"strings"

	"octochan/core"
)

// Builtin — встроенная функция языка.
//
// Все они — тонкие обёртки над core, тем же пакетом, на который опираются
// команды CLI. Благодаря этому diff в языке и diff в командной строке
// считают расхождения одинаково.
type Builtin struct {
	Name string
	// MinArgs и MaxArgs задают арность; MaxArgs = -1 означает «сколько угодно».
	MinArgs int
	MaxArgs int
	Doc     string
	Fn      func(in *Interpreter, args []Value) (Value, error)
}

func (b Builtin) checkArity(n int) error {
	if n < b.MinArgs {
		return fmt.Errorf("нужно минимум %d %s, передано %d",
			b.MinArgs, plural(b.MinArgs, "аргумент", "аргумента", "аргументов"), n)
	}
	if b.MaxArgs >= 0 && n > b.MaxArgs {
		return fmt.Errorf("принимает не более %d %s, передано %d",
			b.MaxArgs, plural(b.MaxArgs, "аргумента", "аргументов", "аргументов"), n)
	}
	return nil
}

// Signature возвращает описание вызова для справки.
func (b Builtin) Signature() string {
	switch {
	case b.MaxArgs < 0:
		return fmt.Sprintf("%s(...)", b.Name)
	case b.MinArgs == b.MaxArgs:
		return fmt.Sprintf("%s/%d", b.Name, b.MinArgs)
	default:
		return fmt.Sprintf("%s/%d..%d", b.Name, b.MinArgs, b.MaxArgs)
	}
}

// DefaultBuiltins возвращает набор встроенных функций.
func DefaultBuiltins() map[string]Builtin {
	list := []Builtin{
		{
			Name: "diff", MinArgs: 2, MaxArgs: 2,
			Doc: "количество расхождений между двумя конфигами",
			Fn:  builtinDiff,
		},
		{
			Name: "diff_list", MinArgs: 2, MaxArgs: 2,
			Doc: "список имён расходящихся параметров",
			Fn:  builtinDiffList,
		},
		{
			Name: "read", MinArgs: 1, MaxArgs: 1,
			Doc: "содержимое файла",
			Fn:  builtinRead,
		},
		{
			Name: "write", MinArgs: 2, MaxArgs: 2,
			Doc: "записать текст в файл",
			Fn:  builtinWrite,
		},
		{
			Name: "exists", MinArgs: 1, MaxArgs: 1,
			Doc: "существует ли файл",
			Fn:  builtinExists,
		},
		{
			Name: "param", MinArgs: 2, MaxArgs: 3,
			Doc: "значение параметра конфига: param(файл, имя) или param(файл, секция, имя)",
			Fn:  builtinParam,
		},
		{
			Name: "params", MinArgs: 1, MaxArgs: 1,
			Doc: "количество параметров в конфиге",
			Fn:  builtinParams,
		},
		{
			Name: "validate", MinArgs: 1, MaxArgs: 1,
			Doc: "проверить конфиг, вернуть число параметров",
			Fn:  builtinValidate,
		},
		{
			Name: "print", MinArgs: 0, MaxArgs: -1,
			Doc: "напечатать значения",
			Fn:  builtinPrint,
		},
		{
			Name: "fail", MinArgs: 0, MaxArgs: -1,
			Doc: "прервать сценарий с сообщением",
			Fn:  builtinFail,
		},
		{
			Name: "len", MinArgs: 1, MaxArgs: 1,
			Doc: "длина списка или строки",
			Fn:  builtinLen,
		},
		{
			Name: "lines", MinArgs: 1, MaxArgs: 1,
			Doc: "разбить текст на список строк",
			Fn:  builtinLines,
		},
		{
			Name: "join", MinArgs: 1, MaxArgs: 2,
			Doc: "склеить список в строку",
			Fn:  builtinJoin,
		},
		{
			Name: "str", MinArgs: 1, MaxArgs: 1,
			Doc: "привести к строке",
			Fn:  builtinStr,
		},
		{
			Name: "int", MinArgs: 1, MaxArgs: 1,
			Doc: "привести к целому",
			Fn:  builtinInt,
		},
		{
			Name: "list", MinArgs: 0, MaxArgs: -1,
			Doc: "собрать список из аргументов",
			Fn:  builtinList,
		},
		{
			Name: "env", MinArgs: 1, MaxArgs: 2,
			Doc: "значение переменной окружения",
			Fn:  builtinEnv,
		},
		{
			Name: "normalize", MinArgs: 1, MaxArgs: 1,
			Doc: "каноническая форма значения конфига: normalize(\"8GB\") == normalize(\"8192MB\")",
			Fn:  builtinNormalize,
		},
	}

	builtins := make(map[string]Builtin, len(list))
	for _, b := range list {
		builtins[b.Name] = b
	}
	return builtins
}

// ---------------------------------------------------------------------------
// Работа с конфигами
// ---------------------------------------------------------------------------

// configOf принимает либо путь к файлу, либо готовый текст конфига.
// Это то, что делает конвейер осмысленным: read(f) | ... отдаёт текст.
func configOf(v Value) (core.ConfigMap, string, error) {
	text, name, err := textOf(v)
	if err != nil {
		return nil, "", err
	}

	cfg, err := core.ParseConfig(text)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", name, err)
	}
	return cfg, name, nil
}

// textOf возвращает текст и человекочитаемое имя источника.
func textOf(v Value) (string, string, error) {
	s, ok := v.(StringValue)
	if !ok {
		return "", "", fmt.Errorf("ожидался путь к файлу или текст, получено %s", v.TypeName())
	}

	path := string(s)

	// Многострочное значение считаем содержимым, а не путём.
	if strings.ContainsAny(path, "\n") {
		return path, "текст", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("не удалось прочитать %s: %w", path, err)
	}
	return string(data), path, nil
}

func builtinDiff(_ *Interpreter, args []Value) (Value, error) {
	left, leftName, err := configOf(args[0])
	if err != nil {
		return nil, err
	}
	right, rightName, err := configOf(args[1])
	if err != nil {
		return nil, err
	}

	return IntValue(core.CompareConfigs(left, right, leftName, rightName).Len()), nil
}

func builtinDiffList(_ *Interpreter, args []Value) (Value, error) {
	left, leftName, err := configOf(args[0])
	if err != nil {
		return nil, err
	}
	right, rightName, err := configOf(args[1])
	if err != nil {
		return nil, err
	}

	diff := core.CompareConfigs(left, right, leftName, rightName)
	names := make(ListValue, 0, diff.Len())
	for _, entry := range diff.Entries {
		names = append(names, StringValue(entry.Param()))
	}
	return names, nil
}

func builtinParam(_ *Interpreter, args []Value) (Value, error) {
	cfg, name, err := configOf(args[0])
	if err != nil {
		return nil, err
	}

	var section, key string
	if len(args) == 3 {
		section, key = args[1].Text(), args[2].Text()
	} else {
		key = args[1].Text()
	}

	value, ok := cfg[section][key]
	if !ok {
		where := "без секции"
		if section != "" {
			where = "секции " + section
		}
		return nil, fmt.Errorf("параметр %q не найден в %s (%s)", key, where, name)
	}
	return StringValue(value), nil
}

func builtinParams(_ *Interpreter, args []Value) (Value, error) {
	cfg, _, err := configOf(args[0])
	if err != nil {
		return nil, err
	}

	total := 0
	for _, section := range cfg {
		total += len(section)
	}
	return IntValue(total), nil
}

func builtinValidate(_ *Interpreter, args []Value) (Value, error) {
	source := Value(StringValue("-"))
	if len(args) == 1 {
		source = args[0]
	}

	cfg, name, err := configOf(source)
	if err != nil {
		return nil, err
	}

	total := 0
	for _, section := range cfg {
		total += len(section)
	}
	if total == 0 {
		return nil, fmt.Errorf("%s: не найдено ни одного параметра", name)
	}
	return IntValue(total), nil
}

func builtinNormalize(_ *Interpreter, args []Value) (Value, error) {
	return StringValue(core.NormalizeValue(args[0].Text())), nil
}

// ---------------------------------------------------------------------------
// Файлы
// ---------------------------------------------------------------------------

func builtinRead(_ *Interpreter, args []Value) (Value, error) {
	data, err := os.ReadFile(args[0].Text())
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать %s: %w", args[0].Text(), err)
	}
	return StringValue(data), nil
}

func builtinWrite(_ *Interpreter, args []Value) (Value, error) {
	path := args[0].Text()
	content := args[1].Text()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("не удалось записать %s: %w", path, err)
	}
	return IntValue(len(content)), nil
}

func builtinExists(_ *Interpreter, args []Value) (Value, error) {
	return BoolValue(core.FileExists(args[0].Text())), nil
}

// ---------------------------------------------------------------------------
// Общего назначения
// ---------------------------------------------------------------------------

func builtinPrint(in *Interpreter, args []Value) (Value, error) {
	if len(args) == 0 {
		fmt.Fprintln(in.Out())
		return StringValue(""), nil
	}

	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = Display(arg)
	}

	line := strings.Join(parts, " ")
	fmt.Fprintln(in.Out(), line)
	return StringValue(line), nil
}

func builtinFail(_ *Interpreter, args []Value) (Value, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("сценарий прерван")
	}

	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = Display(arg)
	}
	return nil, fmt.Errorf("%s", strings.Join(parts, " "))
}

func builtinLen(_ *Interpreter, args []Value) (Value, error) {
	switch v := args[0].(type) {
	case ListValue:
		return IntValue(len(v)), nil
	case StringValue:
		return IntValue(len([]rune(v))), nil
	case NullValue:
		return IntValue(0), nil
	default:
		return nil, fmt.Errorf("len неприменим к %s", args[0].TypeName())
	}
}

func builtinLines(_ *Interpreter, args []Value) (Value, error) {
	text := args[0].Text()
	if strings.TrimSpace(text) == "" {
		return ListValue{}, nil
	}

	raw := strings.Split(strings.TrimRight(text, "\n"), "\n")
	lines := make(ListValue, len(raw))
	for i, line := range raw {
		lines[i] = StringValue(strings.TrimRight(line, "\r"))
	}
	return lines, nil
}

func builtinJoin(_ *Interpreter, args []Value) (Value, error) {
	list, ok := args[0].(ListValue)
	if !ok {
		return nil, fmt.Errorf("join ожидает список, получено %s", args[0].TypeName())
	}

	sep := ", "
	if len(args) == 2 {
		sep = args[1].Text()
	}

	parts := make([]string, len(list))
	for i, item := range list {
		parts[i] = item.Text()
	}
	return StringValue(strings.Join(parts, sep)), nil
}

func builtinStr(_ *Interpreter, args []Value) (Value, error) {
	return StringValue(args[0].Text()), nil
}

func builtinInt(_ *Interpreter, args []Value) (Value, error) {
	if n, ok := numeric(args[0]); ok {
		return IntValue(int64(n)), nil
	}
	return nil, fmt.Errorf("не удалось привести %s %q к целому",
		args[0].TypeName(), args[0].Text())
}

func builtinList(_ *Interpreter, args []Value) (Value, error) {
	items := make(ListValue, len(args))
	copy(items, args)
	return items, nil
}

func builtinEnv(_ *Interpreter, args []Value) (Value, error) {
	value, ok := os.LookupEnv(args[0].Text())
	if ok {
		return StringValue(value), nil
	}
	if len(args) == 2 {
		return args[1], nil
	}
	return StringValue(""), nil
}
