package output

import (
	"reflect"

	"github.com/geoffmcc/nodex/internal/redact"
)

// sanitizeTerminalData returns a deep copy of v where string values have been
// regex-redacted and stripped of terminal-control sequences before structured
// serialization. This keeps JSON/YAML valid while preventing hostile strings
// from being encoded as terminal-active content for downstream renderers.
func sanitizeTerminalData(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	return sanitizeTerminalValue(rv)
}

func sanitizeTerminalValue(rv reflect.Value) any {
	if !rv.IsValid() {
		return nil
	}
	if rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		return sanitizeTerminalValue(rv.Elem())
	}
	if !rv.CanInterface() {
		return nil
	}

	switch rv.Kind() {
	case reflect.String:
		return SanitizeTerminal(redact.String(rv.String()))
	case reflect.Map:
		if rv.IsNil() {
			return nil
		}
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			key := sanitizeTerminalValue(iter.Key())
			val := sanitizeTerminalValue(iter.Value())
			if key == nil || val == nil {
				continue
			}
			kv := reflect.ValueOf(key)
			vv := reflect.ValueOf(val)
			if kv.Type().AssignableTo(rv.Type().Key()) && vv.Type().AssignableTo(rv.Type().Elem()) {
				out.SetMapIndex(kv, vv)
			}
		}
		return out.Interface()
	case reflect.Slice:
		if rv.IsNil() {
			return nil
		}
		out := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Cap())
		for i := 0; i < rv.Len(); i++ {
			val := sanitizeTerminalValue(rv.Index(i))
			if val == nil {
				continue
			}
			vv := reflect.ValueOf(val)
			if vv.Type().AssignableTo(rv.Type().Elem()) {
				out.Index(i).Set(vv)
			}
		}
		return out.Interface()
	case reflect.Array:
		out := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.Len(); i++ {
			val := sanitizeTerminalValue(rv.Index(i))
			if val == nil {
				continue
			}
			vv := reflect.ValueOf(val)
			if vv.Type().AssignableTo(rv.Type().Elem()) {
				out.Index(i).Set(vv)
			}
		}
		return out.Interface()
	case reflect.Struct:
		out := reflect.New(rv.Type()).Elem()
		for i := 0; i < rv.NumField(); i++ {
			field := rv.Field(i)
			dst := out.Field(i)
			if !field.CanInterface() || !dst.CanSet() {
				continue
			}
			val := sanitizeTerminalValue(field)
			if val == nil {
				continue
			}
			vv := reflect.ValueOf(val)
			if dst.Kind() == reflect.Ptr && vv.Kind() != reflect.Ptr {
				ptr := reflect.New(vv.Type())
				ptr.Elem().Set(vv)
				vv = ptr
			}
			if dst.Kind() != reflect.Ptr && vv.Kind() == reflect.Ptr && !vv.IsNil() {
				vv = vv.Elem()
			}
			if vv.Type().AssignableTo(dst.Type()) {
				dst.Set(vv)
			}
		}
		return out.Interface()
	default:
		return rv.Interface()
	}
}
