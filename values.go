package values

import (
	"encoding"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/halliday/go-errors"
	"github.com/halliday/go-tools/stringtools"
)

type ValuesUnmarshaler interface {
	UnmarshalValues(u url.Values) error
}

func Unmarshal(u url.Values, i interface{}) error {

	if len(u) == 0 {
		return nil
	}

	v := reflect.ValueOf(i)
	for {
		if unmarshaler, ok := v.Interface().(ValuesUnmarshaler); ok {
			return unmarshaler.UnmarshalValues(u)
		}

		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
			continue
		}
		break
	}

	if v.Kind() != reflect.Struct {
		return errors.NewCode(400, "values.Unmarshal: unsupported %q", v.Type())
	}

	knownFields := make(map[string]struct{}, v.Type().NumField())
	if err := unmarshalStruct(knownFields, u, v); err != nil {
		return err
	}
	for name := range u {
		if _, ok := knownFields[name]; !ok {
			return errors.NewCode(400, "unknown query: %q", name)
		}
	}
	return nil
}

func deepCreate(v reflect.Value) reflect.Value {
	if v.Kind() != reflect.Ptr {
		return v
	}
	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return deepCreate(v.Elem())
}

func unmarshalStruct(knownFields map[string]struct{}, u url.Values, v reflect.Value) error {
	t := v.Type()
	for n := 0; n < t.NumField(); n++ {
		fv := v.Field(n)
		ft := t.Field(n)
		var name string
		query, ok := ft.Tag.Lookup("query")
		if ok {
			name = query
			if name == "-" || name == "" {
				continue
			}
			if name == "*" {
				name = stringtools.CamelToSnake(ft.Name)
			}
		} else {
			if ft.Anonymous {
				fv = deepCreate(fv)
				if err := unmarshalStruct(knownFields, u, fv); err != nil {
					return err
				}
				continue
			}
			name = stringtools.CamelToSnake(ft.Name)
		}
		knownFields[name] = struct{}{}
		if w, ok := u[name]; ok {
			if err := unmarshalField(name, fv, w); err != nil {
				return err
			}
		}
	}
	return nil
}

type StringsParser interface {
	ParseStrings(s []string) error
}

func unmarshalField(name string, v reflect.Value, w []string) (err error) {
	t := v.Type()

	u, ok := v.Addr().Interface().(StringsParser)
	if ok {
		err = u.ParseStrings(w)
		if err != nil {
			return errors.NewCode(400, "value %q: %v", w, err)
		}
		return nil
	}

	switch v.Kind() {
	case reflect.Int, reflect.Int64, reflect.String:
		return unmarshalSingle(name, v, w)

	case reflect.Bool:
		if len(w) != 1 || w[0] != "" {
			return errors.NewCode(400, "value %q: boolean must not have value", name)
		}
		v.SetBool(true)

	case reflect.Slice:
		// if len(w) == 0 {
		// 	v.SetLen(0)
		// 	break
		// }
		if len(w) != 1 {
			return errors.NewCode(400, "value %q: multiple values", name)
		}
		v.SetLen(0)
		o := w[0]
		n := strings.IndexRune(o, ',')
		i := 0

		for n != -1 {
			x := reflect.New(t.Elem()).Elem()
			if err := unmarshalString(name+"["+strconv.Itoa(i)+"]", x, o[:n]); err != nil {
				return err
			}
			v.Set(reflect.Append(v, x))
			o = o[n+1:]
			n = strings.IndexRune(o, ',')
			i++
		}

		x := reflect.New(t.Elem()).Elem()
		if err := unmarshalString(name+"["+strconv.Itoa(i)+"]", x, o); err != nil {
			return err
		}
		v.Set(reflect.Append(v, x))

	default:
		return unmarshalSingle(name, v, w)
		// switch t {
		// case timeDurationType, timeTimeType:

		// default:
		// 	return errors.Code(400, "value %q: unsupported type %q", name, t)
		// }
	}
	return nil
}

func unmarshalSingle(name string, v reflect.Value, values []string) error {
	if len(values) == 0 {
		return errors.NewCode(400, "value %q: missing value", name)
	}
	if len(values) != 1 {
		return errors.NewCode(400, "value %q: multiple values", name)
	}
	return unmarshalString(name, v, values[0])
}

type StringParser interface {
	ParseString(s string) error
}

var timeDurationType = reflect.TypeOf((*time.Duration)(nil)).Elem()
var googleUUIDType = reflect.TypeOf((*uuid.UUID)(nil)).Elem()

func unmarshalString(name string, v reflect.Value, w string) error {

	stringParser, ok := v.Addr().Interface().(StringParser)
	if ok {
		err := stringParser.ParseString(w)
		if err != nil {
			return errors.NewCode(400, "value %q: %s", name, err)
		}
		return nil
	}

	textUnmarshaler, ok := v.Addr().Interface().(encoding.TextUnmarshaler)
	if ok {
		err := textUnmarshaler.UnmarshalText([]byte(w))
		if err != nil {
			return errors.NewCode(400, "value %q: %s", name, err)
		}
		return nil
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(w)

	case reflect.Int:
		i, err := strconv.Atoi(w)
		if err != nil {
			return errors.NewCode(400, "value %q: must be an integer", name)
		}
		v.SetInt(int64(i))

	case reflect.Int64:
		i, err := strconv.ParseInt(w, 10, 64)
		if err != nil {
			return errors.NewCode(400, "value %q: must be an integer", name)
		}
		v.SetInt(i)
	case reflect.Bool:
		v.SetBool(true)

	default:
		switch v.Type() {
		case timeDurationType:
			d, err := time.ParseDuration(w)
			if err != nil {
				return errors.NewCode(400, "value %q: must be an time duration", name)
			}
			v.Set(reflect.ValueOf(d))

		case googleUUIDType:
			id, err := uuid.Parse(w)
			if err != nil {
				return errors.NewCode(400, "value %q: must be a UUID", name)
			}
			v.Set(reflect.ValueOf(id))

		default:
			return errors.NewCode(400, "value %q: unsupported type %q", name, v.Type())
		}
	}
	return nil
}
