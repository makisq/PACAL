package lang

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run выполняет исходник и возвращает напечатанное.
func run(t *testing.T, src string) (string, error) {
	t.Helper()

	file, err := Parse("<тест>", src)
	if err != nil {
		return "", err
	}

	registry := NewRegistry()
	for _, decl := range file.Aliases {
		if err := registry.Add("", decl); err != nil {
			return "", err
		}
	}

	var out strings.Builder
	interp := NewInterpreter(context.Background(), registry, &out)
	// Выполнение обязано пройти до чтения буфера: в записи вида
	// `return out.String(), interp.RunFile(file)` Go вычисляет out.String()
	// первым и всегда отдаёт пустую строку.
	err = interp.RunFile(file)
	return out.String(), err
}

// mustRun требует успешного выполнения.
func mustRun(t *testing.T, src string) string {
	t.Helper()
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("выполнение не удалось: %v\nисходник:\n%s", err, src)
	}
	return out
}

func TestArithmeticAndComparison(t *testing.T) {
	cases := map[string]string{
		`print(2 + 3 * 4)`:        "14",
		`print((2 + 3) * 4)`:      "20",
		`print(7 / 2)`:            "3.5",
		`print(7 % 2)`:            "1",
		`print(-5 + 2)`:           "-3",
		`print(1 < 2)`:            "true",
		`print(2 <= 2)`:           "true",
		`print("a" + "b")`:        "ab",
		`print(1 == 1.0)`:         "true",
		`print("8" == 8)`:         "true",
		`print(not 0)`:            "true",
		`print(true and false)`:   "false",
		`print(false or 1)`:       "true",
		`print([1, 2] + [3])`:     "[1, 2, 3]",
		`print(len([1, 2, 3]))`:   "3",
		`print([10, 20, 30][1])`:  "20",
		`print([10, 20, 30][-1])`: "30",
	}

	for src, want := range cases {
		t.Run(src, func(t *testing.T) {
			got := strings.TrimSpace(mustRun(t, src))
			if got != want {
				t.Errorf("получено %q, ожидалось %q", got, want)
			}
		})
	}
}

func TestShortCircuit(t *testing.T) {
	// Правая часть and не должна вычисляться, если левая ложна:
	// иначе fail() прервал бы сценарий.
	if _, err := run(t, `print(false and fail("не должно выполниться"))`); err != nil {
		t.Errorf("and вычислил правую часть: %v", err)
	}
	if _, err := run(t, `print(true or fail("не должно выполниться"))`); err != nil {
		t.Errorf("or вычислил правую часть: %v", err)
	}
}

func TestIfElseChain(t *testing.T) {
	src := `
x = 5
if x > 10 {
    print("больше")
} else if x > 3 {
    print("средне")
} else {
    print("меньше")
}
`
	if got := strings.TrimSpace(mustRun(t, src)); got != "средне" {
		t.Errorf("получено %q", got)
	}
}

func TestForLoop(t *testing.T) {
	src := `
сумма = 0
for n in [1, 2, 3, 4] {
    if n == 3 { continue }
    if n == 4 { break }
    сумма = сумма + n
}
print(сумма)
`
	if got := strings.TrimSpace(mustRun(t, src)); got != "3" {
		t.Errorf("получено %q, ожидалось 3", got)
	}
}

func TestForOverLines(t *testing.T) {
	// Строка обходится построчно — это делает конвейер полезным в цикле.
	src := `
for строка in lines("a\nb\nc") {
    print(строка)
}
`
	got := strings.TrimSpace(mustRun(t, src))
	if got != "a\nb\nc" {
		t.Errorf("получено %q", got)
	}
}

func TestAliasWithParamsAndRecursion(t *testing.T) {
	src := `
alias факториал(n) {
    if n <= 1 { return 1 }
    return n * факториал(n - 1)
}
print(факториал(5))
`
	if got := strings.TrimSpace(mustRun(t, src)); got != "120" {
		t.Errorf("получено %q, ожидалось 120", got)
	}
}

func TestAliasCallsAlias(t *testing.T) {
	src := `
alias удвоить(x) { return x * 2 }
alias учетверить(x) { return удвоить(удвоить(x)) }
print(учетверить(3))
`
	if got := strings.TrimSpace(mustRun(t, src)); got != "12" {
		t.Errorf("получено %q, ожидалось 12", got)
	}
}

func TestInfiniteRecursionIsStopped(t *testing.T) {
	// Без ограничения глубины это повесило бы процесс.
	_, err := run(t, `
alias бесконечность() { return бесконечность() }
бесконечность()
`)
	if err == nil {
		t.Fatal("ожидалась ошибка о слишком глубокой рекурсии")
	}
	if !strings.Contains(err.Error(), "рекурси") {
		t.Errorf("непонятное сообщение: %v", err)
	}
}

func TestAliasDoesNotSeeCallerVariables(t *testing.T) {
	// Изоляция окружения: иначе алиасы из чужого пакета могли бы
	// незаметно зависеть от переменных вызывающего.
	_, err := run(t, `
alias показать() { return секрет }
секрет = 42
показать()
`)
	if err == nil {
		t.Fatal("алиас увидел переменную вызывающего")
	}
}

func TestArityMismatch(t *testing.T) {
	_, err := run(t, `
alias пара(a, b) { return a }
пара(1)
`)
	if err == nil || !strings.Contains(err.Error(), "принимает 2") {
		t.Fatalf("ожидалась ошибка об арности, получено: %v", err)
	}
}

func TestPipeline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.conf")
	if err := os.WriteFile(path, []byte("shared_buffers = 8GB\nmax_connections = 100\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// read даёт текст, validate принимает его из конвейера.
	src := `print(read("` + filepath.ToSlash(path) + `") | validate())`
	if got := strings.TrimSpace(mustRun(t, src)); got != "2" {
		t.Errorf("получено %q, ожидалось 2", got)
	}
}

func TestPipeBindsTighterThanComparison(t *testing.T) {
	// a | b != 0 должно читаться как (a | b) != 0.
	src := `print(lines("x\ny") | len() != 0)`
	if got := strings.TrimSpace(mustRun(t, src)); got != "true" {
		t.Errorf("получено %q, ожидалось true", got)
	}
}

func TestDiffBuiltin(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "prod.conf")
	right := filepath.Join(dir, "test.conf")

	// 8GB и 8192MB равны, max_connections различается.
	if err := os.WriteFile(left, []byte("shared_buffers = 8GB\nmax_connections = 100\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(right, []byte("shared_buffers = 8192MB\nmax_connections = 200\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := `print(diff("` + filepath.ToSlash(left) + `", "` + filepath.ToSlash(right) + `"))`
	if got := strings.TrimSpace(mustRun(t, src)); got != "1" {
		t.Errorf("получено %q, ожидалось 1 расхождение", got)
	}
}

func TestNamespaces(t *testing.T) {
	registry := NewRegistry()

	userFile, err := Parse("user.ochan", `alias проверить() { return "пользовательский" }`)
	if err != nil {
		t.Fatal(err)
	}
	pkgFile, err := Parse("devops.ochan", `alias проверить() { return "из пакета" }`)
	if err != nil {
		t.Fatal(err)
	}

	if err := registry.Add("", userFile.Aliases[0]); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("devops", pkgFile.Aliases[0]); err != nil {
		t.Fatal(err)
	}

	// Короткое имя отдаёт пользовательский алиас.
	decl, err := registry.Lookup("", "проверить")
	if err != nil {
		t.Fatal(err)
	}
	if decl.Module != "" {
		t.Errorf("короткое имя ушло в пакет %q", decl.Module)
	}

	// Квалифицированное — из пакета.
	decl, err = registry.Lookup("devops", "проверить")
	if err != nil {
		t.Fatal(err)
	}
	if decl.Module != "devops" {
		t.Errorf("получен алиас из %q", decl.Module)
	}
}

func TestAmbiguousShortNameIsError(t *testing.T) {
	// Два пакета с одинаковым именем алиаса не должны разрешаться молча.
	registry := NewRegistry()
	for _, pkg := range []string{"пакет_а", "пакет_б"} {
		file, err := Parse(pkg+".ochan", `alias общий() { return 1 }`)
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Add(pkg, file.Aliases[0]); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := registry.Lookup("", "общий"); err == nil {
		t.Fatal("неоднозначное имя разрешилось без ошибки")
	} else if !strings.Contains(err.Error(), "уточните пакет") {
		t.Errorf("непонятное сообщение: %v", err)
	}
}

func TestDuplicateAliasInSameSpace(t *testing.T) {
	registry := NewRegistry()
	file, err := Parse("dup.ochan", "alias повтор() { return 1 }\nalias повтор() { return 2 }")
	if err != nil {
		t.Fatal(err)
	}

	if err := registry.Add("", file.Aliases[0]); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("", file.Aliases[1]); err == nil {
		t.Fatal("повторное объявление прошло без ошибки")
	}
}

func TestParseErrorsAreLocated(t *testing.T) {
	cases := map[string]string{
		"незакрытый блок":      "alias a() {",
		"число с единицами":    `x = 8GB`,
		"двойной амперсанд":    `x = 1 && 2`,
		"восклицательный знак": `x = !1`,
		"лишнее после команды": "print(1) print(2)",
		"незакрытая строка":    `x = "abc`,
		"алиас внутри блока":   "if true { alias a() { return 1 } }",
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := run(t, src); err == nil {
				t.Fatalf("ожидалась ошибка разбора для %q", src)
			}
		})
	}
}

func TestUnknownNameSuggestsAlternative(t *testing.T) {
	_, err := run(t, `prnt("привет")`)
	if err == nil {
		t.Fatal("ожидалась ошибка о неизвестной функции")
	}
	if !strings.Contains(err.Error(), "print") {
		t.Errorf("нет подсказки про print: %v", err)
	}
}

func TestErrorMentionsPositionAndStack(t *testing.T) {
	_, err := run(t, `
alias внешний() { return внутренний() }
alias внутренний() { return fail("сломалось") }
внешний()
`)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}

	msg := err.Error()
	for _, want := range []string{"сломалось", "внешний", "внутренний"} {
		if !strings.Contains(msg, want) {
			t.Errorf("в сообщении нет %q: %s", want, msg)
		}
	}
}

func TestContextCancellationStopsLoop(t *testing.T) {
	file, err := Parse("<тест>", `
alias бесконечный_цикл() {
    for x in [1, 2, 3] { print(x) }
}
бесконечный_цикл()
`)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменяем заранее

	var out strings.Builder
	interp := NewInterpreter(ctx, NewRegistry(), &out)
	for _, decl := range file.Aliases {
		if err := interp.registry.Add("", decl); err != nil {
			t.Fatal(err)
		}
	}

	if err := interp.RunFile(file); err == nil {
		t.Fatal("отменённый контекст не прервал выполнение")
	}
}
