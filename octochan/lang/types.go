package lang

import (
	"fmt"
	"sort"
	"strings"
)

// TypeName — имя объявленного типа.
//
// Аннотации необязательны: без них поведение прежнее, а написанный тип
// проверяется при каждом присваивании и при передаче аргумента. Смысл
// в том, чтобы ошибка всплывала на входе в алиас, а не в его середине —
// особенно когда алиас пришёл из чужого пакета и его тело читать неудобно.
type TypeName string

const (
	TypeAny    TypeName = "any"
	TypeInt    TypeName = "int"
	TypeFloat  TypeName = "float"
	TypeNumber TypeName = "number"
	TypeString TypeName = "string"
	TypeBool   TypeName = "bool"
	TypeList   TypeName = "list"
	TypeMap    TypeName = "map"
	TypeConfig TypeName = "config"
)

// typeAliases позволяет писать имена типов и латиницей, и по-русски:
// ключевые слова языка латинские, но идентификаторы чаще русские,
// и заставлять переключать раскладку посреди строки незачем.
var typeAliases = map[string]TypeName{
	"any":        TypeAny,
	"любой":      TypeAny,
	"int":        TypeInt,
	"целое":      TypeInt,
	"float":      TypeFloat,
	"дробное":    TypeFloat,
	"number":     TypeNumber,
	"число":      TypeNumber,
	"string":     TypeString,
	"строка":     TypeString,
	"bool":       TypeBool,
	"логическое": TypeBool,
	"list":       TypeList,
	"список":     TypeList,
	"map":        TypeMap,
	"dict":       TypeMap,
	"словарь":    TypeMap,
	"config":     TypeConfig,
	"конфиг":     TypeConfig,
}

// ParseTypeName распознаёт имя типа.
func ParseTypeName(name string) (TypeName, error) {
	if t, ok := typeAliases[strings.ToLower(name)]; ok {
		return t, nil
	}
	return "", fmt.Errorf("неизвестный тип %q; доступны: %s", name, strings.Join(knownTypeNames(), ", "))
}

func knownTypeNames() []string {
	seen := make(map[TypeName]bool)
	var names []string
	for name, t := range typeAliases {
		// Показываем по одному написанию на тип, чтобы список не двоился.
		if !seen[t] {
			seen[t] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// Russian возвращает имя типа для сообщений об ошибках.
func (t TypeName) Russian() string {
	switch t {
	case TypeAny:
		return "любой"
	case TypeInt:
		return "целое"
	case TypeFloat:
		return "дробное"
	case TypeNumber:
		return "число"
	case TypeString:
		return "строка"
	case TypeBool:
		return "логическое"
	case TypeList:
		return "список"
	case TypeMap:
		return "словарь"
	case TypeConfig:
		return "конфиг"
	default:
		return string(t)
	}
}

// Accepts сообщает, подходит ли значение объявленному типу.
//
// Единственное послабление — целое там, где объявлено дробное или число:
// расширение без потери данных. Строка числом не считается, иначе
// аннотация не ловила бы ровно ту ошибку, ради которой её пишут.
func (t TypeName) Accepts(v Value) bool {
	if t == TypeAny || t == "" {
		return true
	}

	switch t {
	case TypeInt:
		_, ok := v.(IntValue)
		return ok
	case TypeFloat:
		switch v.(type) {
		case FloatValue, IntValue:
			return true
		}
		return false
	case TypeNumber:
		switch v.(type) {
		case IntValue, FloatValue:
			return true
		}
		return false
	case TypeString:
		_, ok := v.(StringValue)
		return ok
	case TypeBool:
		_, ok := v.(BoolValue)
		return ok
	case TypeList:
		_, ok := v.(ListValue)
		return ok
	case TypeMap:
		_, ok := v.(MapValue)
		return ok
	case TypeConfig:
		_, ok := v.(ConfigValue)
		return ok
	}
	return false
}

// checkType проверяет значение и возвращает понятную ошибку.
func checkType(declared TypeName, what string, v Value) error {
	if declared.Accepts(v) {
		return nil
	}
	// Формулировка нейтральна по роду: what бывает и переменной,
	// и параметром, и элементом цикла.
	return fmt.Errorf("%s: объявлен тип %s, получено значение типа %s (%s)",
		what, declared.Russian(), v.TypeName(), Display(v))
}
