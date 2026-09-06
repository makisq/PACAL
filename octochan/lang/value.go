package lang

import (
	"fmt"
	"strconv"
	"strings"
)

// Value — значение языка.
//
// Модель намеренно разводит два понятия, которые в наброске языка были
// смешаны: «операция не выполнилась» — это ошибка, прерывающая сценарий,
// а не число, а Value — то, что операция вычислила. Поэтому diff возвращает
// количество расхождений, и условие пишется как diff(a, b) != 0.
type Value interface {
	// Text — представление значения в конвейере и при печати.
	Text() string
	// Truthy — истинность в условиях.
	Truthy() bool
	// TypeName — имя типа для сообщений об ошибках.
	TypeName() string
}

// NullValue — отсутствие значения.
type NullValue struct{}

func (NullValue) Text() string     { return "" }
func (NullValue) Truthy() bool     { return false }
func (NullValue) TypeName() string { return "null" }

// BoolValue — логическое значение.
type BoolValue bool

func (v BoolValue) Text() string {
	if v {
		return "true"
	}
	return "false"
}
func (v BoolValue) Truthy() bool   { return bool(v) }
func (BoolValue) TypeName() string { return "логическое" }

// IntValue — целое число.
type IntValue int64

func (v IntValue) Text() string   { return strconv.FormatInt(int64(v), 10) }
func (v IntValue) Truthy() bool   { return v != 0 }
func (IntValue) TypeName() string { return "целое" }

// FloatValue — дробное число.
type FloatValue float64

func (v FloatValue) Text() string   { return strconv.FormatFloat(float64(v), 'f', -1, 64) }
func (v FloatValue) Truthy() bool   { return v != 0 }
func (FloatValue) TypeName() string { return "дробное" }

// StringValue — строка.
type StringValue string

func (v StringValue) Text() string   { return string(v) }
func (v StringValue) Truthy() bool   { return len(v) > 0 }
func (StringValue) TypeName() string { return "строка" }

// ListValue — список значений.
type ListValue []Value

func (v ListValue) Text() string {
	parts := make([]string, len(v))
	for i, item := range v {
		parts[i] = item.Text()
	}
	// Перевод строки как разделитель выбран ради конвейеров: список,
	// отданный в текстовую операцию, выглядит как построчный вывод.
	return strings.Join(parts, "\n")
}
func (v ListValue) Truthy() bool   { return len(v) > 0 }
func (ListValue) TypeName() string { return "список" }

// Display — человекочитаемое представление для печати и отладки.
func Display(v Value) string {
	switch val := v.(type) {
	case NullValue:
		return "null"
	case StringValue:
		return string(val)
	case ListValue:
		parts := make([]string, len(val))
		for i, item := range val {
			parts[i] = Display(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return v.Text()
	}
}

// numeric приводит значение к числу, если это возможно без потери смысла.
func numeric(v Value) (float64, bool) {
	switch val := v.(type) {
	case IntValue:
		return float64(val), true
	case FloatValue:
		return float64(val), true
	case BoolValue:
		if val {
			return 1, true
		}
		return 0, true
	case StringValue:
		f, err := strconv.ParseFloat(strings.TrimSpace(string(val)), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func isInt(v Value) bool {
	_, ok := v.(IntValue)
	return ok
}

// equalValues сравнивает значения: числа — численно, остальное — по тексту.
func equalValues(a, b Value) bool {
	if _, aNull := a.(NullValue); aNull {
		_, bNull := b.(NullValue)
		return bNull
	}
	if _, bNull := b.(NullValue); bNull {
		return false
	}

	if aList, ok := a.(ListValue); ok {
		bList, ok := b.(ListValue)
		if !ok || len(aList) != len(bList) {
			return false
		}
		for i := range aList {
			if !equalValues(aList[i], bList[i]) {
				return false
			}
		}
		return true
	}

	if an, aok := numeric(a); aok {
		if bn, bok := numeric(b); bok {
			return an == bn
		}
	}
	return a.Text() == b.Text()
}

// compareValues возвращает -1, 0 или 1. Ошибка означает несравнимые типы.
func compareValues(a, b Value) (int, error) {
	if an, aok := numeric(a); aok {
		if bn, bok := numeric(b); bok {
			switch {
			case an < bn:
				return -1, nil
			case an > bn:
				return 1, nil
			default:
				return 0, nil
			}
		}
	}

	as, aok := a.(StringValue)
	bs, bok := b.(StringValue)
	if aok && bok {
		return strings.Compare(string(as), string(bs)), nil
	}

	return 0, fmt.Errorf("нельзя сравнить %s и %s", a.TypeName(), b.TypeName())
}

// FromText превращает текст в значение, распознавая числа.
// Используется на границе с конвейером и встроенными функциями.
func FromText(s string) Value {
	trimmed := strings.TrimSpace(s)
	if trimmed != "" {
		if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return IntValue(n)
		}
	}
	return StringValue(s)
}
