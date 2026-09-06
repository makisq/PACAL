package module

import (
	"reflect"
	"testing"
)

func TestParamsBool(t *testing.T) {
	p := Params{
		"native":     true,
		"json_num":   float64(1), // JSON отдаёт числа как float64
		"yaml_num":   1,          // YAML — как int
		"str_true":   "TRUE",
		"str_on":     "on",
		"str_false":  "no",
		"str_junk":   "возможно",
		"zero_float": float64(0),
	}

	cases := map[string]bool{
		"native":     true,
		"json_num":   true, // прежняя версия не знала float64 и возвращала default
		"yaml_num":   true,
		"str_true":   true,
		"str_on":     true,
		"str_false":  false,
		"zero_float": false,
	}

	for key, want := range cases {
		if got := p.Bool(key, false); got != want {
			t.Errorf("Bool(%q) = %v, ожидалось %v", key, got, want)
		}
	}

	if got := p.Bool("str_junk", true); !got {
		t.Error("нераспознанное значение должно давать значение по умолчанию")
	}
	if got := p.Bool("missing", true); !got {
		t.Error("отсутствующий ключ должен давать значение по умолчанию")
	}
}

func TestParamsStringList(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []string
	}{
		// Раньше здесь было Parameters["config_list"].(string) без проверки:
		// список из YAML ронял процесс паникой.
		{"список", []any{"a", "b"}, []string{"a", "b"}},
		{"строка", "a, b ,c", []string{"a", "b", "c"}},
		{"пустая строка", "", nil},
		{"числа в списке", []any{1, 2}, []string{"1", "2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Params{"config_list": tt.value}
			got := p.StringList("config_list")
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("получено %#v, ожидалось %#v", got, tt.want)
			}
		})
	}

	if got := (Params{}).StringList("нет"); got != nil {
		t.Errorf("отсутствующий ключ должен давать nil, получено %#v", got)
	}
}

func TestParamsInt(t *testing.T) {
	p := Params{"json": float64(5433), "yaml": 5433, "str": "5433", "junk": "порт"}

	for _, key := range []string{"json", "yaml", "str"} {
		if got := p.Int(key, 5432); got != 5433 {
			t.Errorf("Int(%q) = %d, ожидалось 5433", key, got)
		}
	}
	if got := p.Int("junk", 5432); got != 5432 {
		t.Errorf("Int(junk) = %d, ожидалось значение по умолчанию 5432", got)
	}
}

func TestParamsRequireNonEmptyDoesNotPanic(t *testing.T) {
	// Прежняя проверка писалась как `val == ""` по interface{} и падала
	// с "comparing uncomparable type" на срезах и map.
	p := Params{"role": []any{"a", "b"}, "port": 5432}

	if err := p.RequireNonEmpty("role", "port"); err != nil {
		t.Errorf("непустые значения не должны давать ошибку: %v", err)
	}
	if err := p.RequireNonEmpty("missing"); err == nil {
		t.Error("ожидалась ошибка об отсутствующем параметре")
	}
	if err := (Params{"role": "  "}).RequireNonEmpty("role"); err == nil {
		t.Error("ожидалась ошибка о пустом значении")
	}
}

func TestItemString(t *testing.T) {
	item := map[string]any{
		"str":   "значение",
		"list":  []any{"первое", "второе"},
		"num":   42,
		"empty": nil,
	}

	cases := map[string]string{
		"str":     "значение",
		"list":    "первое",
		"num":     "42",
		"empty":   "",
		"missing": "",
	}

	for key, want := range cases {
		if got := itemString(item, key); got != want {
			t.Errorf("itemString(%q) = %q, ожидалось %q", key, got, want)
		}
	}
}
