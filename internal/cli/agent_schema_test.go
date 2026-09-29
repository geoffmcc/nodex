package cli

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type schemaDocument struct {
	path string
	root any
}

// validateAgainstJSONSchema validates representative command output against
// the JSON Schema subset used by NodeX's checked-in schemas. It covers refs,
// required/properties, type/enum/const, bounds, conditionals, and compositions
// so tests exercise the schema files rather than only Go struct validation.
func validateAgainstJSONSchema(schemaPath string, raw []byte) error {
	doc, err := loadSchemaDocument(schemaPath)
	if err != nil {
		return err
	}
	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		return fmt.Errorf("decode representative JSON: %w", err)
	}
	return validateSchemaValue(instance, doc.root, doc, "$", 0)
}

func loadSchemaDocument(path string) (schemaDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schemaDocument{}, err
	}
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return schemaDocument{}, fmt.Errorf("decode schema %s: %w", path, err)
	}
	return schemaDocument{path: path, root: root}, nil
}

func validateSchemaValue(value, rawSchema any, doc schemaDocument, path string, depth int) error {
	if depth > 128 {
		return fmt.Errorf("schema recursion exceeds bound at %s", path)
	}
	schema, ok := rawSchema.(map[string]any)
	if !ok {
		return fmt.Errorf("schema at %s is not an object", path)
	}
	if ref, ok := schema["$ref"].(string); ok {
		refDoc, target, err := resolveSchemaRef(doc, ref)
		if err != nil {
			return fmt.Errorf("resolve schema ref %q at %s: %w", ref, path, err)
		}
		if err := validateSchemaValue(value, target, refDoc, path, depth+1); err != nil {
			return err
		}
	}
	if want, ok := schema["type"]; ok && !matchesSchemaType(value, want) {
		return fmt.Errorf("%s has type %T, schema requires %v", path, value, want)
	}
	if want, ok := schema["const"]; ok && !reflect.DeepEqual(value, want) {
		return fmt.Errorf("%s=%v, schema constant is %v", path, value, want)
	}
	if enum, ok := schemaStrings(schema["enum"]); ok {
		found := false
		for _, choice := range enum {
			if value == choice {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s=%v is outside enum %v", path, value, enum)
		}
	}
	if allOf, ok := schema["allOf"].([]any); ok {
		for _, sub := range allOf {
			if err := validateSchemaValue(value, sub, doc, path, depth+1); err != nil {
				return err
			}
		}
	}
	if oneOf, ok := schema["oneOf"].([]any); ok {
		valid := 0
		var rejected []string
		for _, sub := range oneOf {
			if err := validateSchemaValue(value, sub, doc, path, depth+1); err == nil {
				valid++
			} else {
				rejected = append(rejected, err.Error())
			}
		}
		if valid != 1 {
			return fmt.Errorf("%s matches %d oneOf alternatives, want exactly one (rejected: %s)", path, valid, strings.Join(rejected, "; "))
		}
	}
	if condition, ok := schema["if"]; ok {
		thenSchema, hasThen := schema["then"]
		elseSchema, hasElse := schema["else"]
		if validateSchemaValue(value, condition, doc, path, depth+1) == nil {
			if hasThen {
				if err := validateSchemaValue(value, thenSchema, doc, path, depth+1); err != nil {
					return err
				}
			}
		} else if hasElse {
			if err := validateSchemaValue(value, elseSchema, doc, path, depth+1); err != nil {
				return err
			}
		}
	}
	if notSchema, ok := schema["not"]; ok && validateSchemaValue(value, notSchema, doc, path, depth+1) == nil {
		return fmt.Errorf("%s matches a forbidden not schema", path)
	}
	if object, ok := value.(map[string]any); ok {
		if required, ok := schemaStrings(schema["required"]); ok {
			for _, field := range required {
				if _, exists := object[field]; !exists {
					return fmt.Errorf("%s is missing required property %q", path, field)
				}
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for name, subSchema := range properties {
			if property, exists := object[name]; exists {
				if err := validateSchemaValue(property, subSchema, doc, path+"."+name, depth+1); err != nil {
					return err
				}
			}
		}
		if additional, exists := schema["additionalProperties"]; exists {
			for name, property := range object {
				if _, known := properties[name]; known {
					continue
				}
				if allowed, isBool := additional.(bool); isBool {
					if !allowed {
						return fmt.Errorf("%s has unexpected property %q", path, name)
					}
				} else if err := validateSchemaValue(property, additional, doc, path+"."+name, depth+1); err != nil {
					return err
				}
			}
		}
	}
	if list, ok := value.([]any); ok {
		if min, ok := schemaNumber(schema["minItems"]); ok && float64(len(list)) < min {
			return fmt.Errorf("%s has %d items; schema minimum is %v", path, len(list), min)
		}
		if max, ok := schemaNumber(schema["maxItems"]); ok && float64(len(list)) > max {
			return fmt.Errorf("%s has %d items; schema maximum is %v", path, len(list), max)
		}
		if prefix, ok := schema["prefixItems"].([]any); ok {
			for i := 0; i < len(list) && i < len(prefix); i++ {
				if err := validateSchemaValue(list[i], prefix[i], doc, fmt.Sprintf("%s[%d]", path, i), depth+1); err != nil {
					return err
				}
			}
		}
		if items, ok := schema["items"]; ok {
			for i := lenPrefixItems(schema); i < len(list); i++ {
				if allowed, isBool := items.(bool); isBool {
					if !allowed {
						return fmt.Errorf("%s[%d] is disallowed by items=false", path, i)
					}
					continue
				}
				if err := validateSchemaValue(list[i], items, doc, fmt.Sprintf("%s[%d]", path, i), depth+1); err != nil {
					return err
				}
			}
		}
	}
	if text, ok := value.(string); ok {
		if minimum, ok := schemaNumber(schema["minLength"]); ok && float64(utf8.RuneCountInString(text)) < minimum {
			return fmt.Errorf("%s is shorter than minLength %v", path, minimum)
		}
		if maximum, ok := schemaNumber(schema["maxLength"]); ok && float64(utf8.RuneCountInString(text)) > maximum {
			return fmt.Errorf("%s is longer than maxLength %v", path, maximum)
		}
		if pattern, ok := schema["pattern"].(string); ok {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("invalid pattern in schema: %w", err)
			}
			if !re.MatchString(text) {
				return fmt.Errorf("%s does not match pattern %q", path, pattern)
			}
		}
		if format, ok := schema["format"].(string); ok {
			switch format {
			case "date-time":
				if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
					return fmt.Errorf("%s is not date-time: %w", path, err)
				}
			case "uri":
				u, err := url.Parse(text)
				if err != nil || u.Scheme == "" || u.Hostname() == "" {
					return fmt.Errorf("%s is not an absolute URI", path)
				}
			}
		}
	}
	if number, ok := value.(float64); ok {
		if minimum, ok := schemaNumber(schema["minimum"]); ok && number < minimum {
			return fmt.Errorf("%s is below minimum %v", path, minimum)
		}
		if maximum, ok := schemaNumber(schema["maximum"]); ok && number > maximum {
			return fmt.Errorf("%s is above maximum %v", path, maximum)
		}
	}
	return nil
}

func resolveSchemaRef(doc schemaDocument, ref string) (schemaDocument, any, error) {
	current := doc
	var fragment string
	if !strings.HasPrefix(ref, "#") {
		filePart, fragmentPart, _ := strings.Cut(ref, "#")
		path := filePart
		if strings.Contains(filePart, "://") {
			u, err := url.Parse(filePart)
			if err != nil {
				return schemaDocument{}, nil, err
			}
			path = filepath.Base(u.Path)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(doc.path), path)
		}
		loaded, err := loadSchemaDocument(path)
		if err != nil {
			return schemaDocument{}, nil, err
		}
		current, fragment = loaded, fragmentPart
	} else {
		fragment = strings.TrimPrefix(ref, "#")
	}
	target := current.root
	if fragment == "" {
		return current, target, nil
	}
	for _, token := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		switch value := target.(type) {
		case map[string]any:
			var ok bool
			target, ok = value[token]
			if !ok {
				return schemaDocument{}, nil, fmt.Errorf("schema pointer component %q not found", token)
			}
		case []any:
			i, err := strconv.Atoi(token)
			if err != nil || i < 0 || i >= len(value) {
				return schemaDocument{}, nil, fmt.Errorf("invalid schema array pointer %q", token)
			}
			target = value[i]
		default:
			return schemaDocument{}, nil, fmt.Errorf("schema pointer component %q traverses a scalar", token)
		}
	}
	return current, target, nil
}

func matchesSchemaType(value, want any) bool {
	if choices, ok := want.([]any); ok {
		for _, choice := range choices {
			if matchesSchemaType(value, choice) {
				return true
			}
		}
		return false
	}
	typeName, ok := want.(string)
	if !ok {
		return false
	}
	switch typeName {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		n, ok := value.(float64)
		return ok && math.Trunc(n) == n
	case "number":
		_, ok := value.(float64)
		return ok
	case "null":
		return value == nil
	default:
		return false
	}
}

func schemaNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case int:
		return float64(number), true
	default:
		return 0, false
	}
}

func schemaStrings(value any) ([]string, bool) {
	switch values := value.(type) {
	case []string:
		return values, true
	case []any:
		stringsOnly := make([]string, 0, len(values))
		for _, item := range values {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			stringsOnly = append(stringsOnly, text)
		}
		return stringsOnly, true
	default:
		return nil, false
	}
}

func lenPrefixItems(schema map[string]any) int {
	items, _ := schema["prefixItems"].([]any)
	return len(items)
}
