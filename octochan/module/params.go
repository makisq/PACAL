package module

import (
	"fmt"
	"strconv"
	"strings"
)

// Params — параметры сценария с типобезопасным доступом.
//
// Раньше getStringParam/getBoolParam/getIntParam были скопированы в
// psql_tuning_params_se.go и restart_pg.go, причём копии расходились:
// одна не понимала float64, из-за чего confirm: 1 из JSON молча
// превращался в false.
type Params map[string]any

// Has сообщает, задан ли параметр.
func (p Params) Has(key string) bool {
	_, ok := p[key]
	return ok
}

// String возвращает строковое значение параметра.
func (p Params) String(key, def string) string {
	v, ok := p[key]
	if !ok || v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// Bool приводит значение к булеву.
// JSON отдаёт числа как float64, YAML — как int, поэтому оба случая
// обрабатываются явно.
func (p Params) Bool(key string, def bool) bool {
	v, ok := p[key]
	if !ok || v == nil {
		return def
	}

	switch val := v.(type) {
	case bool:
		return val
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "true", "1", "yes", "on", "enable", "enabled":
			return true
		case "false", "0", "no", "off", "disable", "disabled":
			return false
		}
	}
	return def
}

// Int приводит значение к целому.
func (p Params) Int(key string, def int) int {
	v, ok := p[key]
	if !ok || v == nil {
		return def
	}

	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return n
		}
	}
	return def
}

// StringList принимает и список, и строку с разделителем-запятой.
// Прежний код делал Parameters["config_list"].(string) без проверки
// и падал, если в YAML был написан список.
func (p Params) StringList(key string) []string {
	v, ok := p[key]
	if !ok || v == nil {
		return nil
	}

	switch val := v.(type) {
	case []string:
		return trimAll(val)
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if item == nil {
				continue
			}
			out = append(out, strings.TrimSpace(fmt.Sprintf("%v", item)))
		}
		return out
	case string:
		if strings.TrimSpace(val) == "" {
			return nil
		}
		return trimAll(strings.Split(val, ","))
	default:
		return []string{fmt.Sprintf("%v", val)}
	}
}

// RequireNonEmpty проверяет, что все перечисленные параметры заданы и непусты.
//
// Прежняя проверка писалась как `val == ""` по interface{} — она не ловила
// пустые списки и падала с "comparing uncomparable type" на map/slice.
func (p Params) RequireNonEmpty(keys ...string) error {
	for _, key := range keys {
		v, ok := p[key]
		if !ok || v == nil {
			return fmt.Errorf("отсутствует обязательный параметр: %s", key)
		}
		if s, isStr := v.(string); isStr && strings.TrimSpace(s) == "" {
			return fmt.Errorf("параметр %s не может быть пустым", key)
		}
	}
	return nil
}

// RequirePresent проверяет только наличие параметров, не заглядывая в значение.
func (p Params) RequirePresent(keys ...string) error {
	for _, key := range keys {
		if !p.Has(key) {
			return fmt.Errorf("отсутствует обязательный параметр: %s", key)
		}
	}
	return nil
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.TrimSpace(s))
	}
	return out
}

// itemString достаёт строковое поле из элемента items.
func itemString(item map[string]any, key string) string {
	v, ok := item[key]
	if !ok || v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case []any:
		if len(val) > 0 {
			if s, ok := val[0].(string); ok {
				return s
			}
		}
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}
