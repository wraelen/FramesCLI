package contracts

import (
	"fmt"
	"reflect"
	"strings"
)

type schemaProvider interface {
	JSONSchema() map[string]any
}

type schemaField struct {
	Name     string
	Schema   map[string]any
	Required bool
}

func (FPSSpec) JSONSchema() map[string]any {
	return map[string]any{
		"anyOf": []map[string]any{
			{"type": "number", "minimum": 0},
			{"type": "string", "enum": []string{"auto"}},
		},
	}
}

func inputSchemaFromArgs(args any) map[string]any {
	fields := schemaFieldsFromArgs(args)
	properties := make(map[string]any, len(fields))
	required := make([]string, 0, len(fields))
	for _, field := range fields {
		properties[field.Name] = field.Schema
		if field.Required {
			required = append(required, field.Name)
		}
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func schemaFieldsFromArgs(args any) []schemaField {
	t := reflect.TypeOf(args)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}

	fields := make([]schemaField, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := jsonFieldName(field)
		if name == "" {
			continue
		}
		fields = append(fields, schemaField{
			Name:     name,
			Schema:   schemaForType(field.Type),
			Required: strings.Contains(field.Tag.Get("contract"), "required"),
		})
	}
	return fields
}

func schemaForType(t reflect.Type) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if schema, ok := providedSchemaForType(t); ok {
		return schema
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": schemaForType(t.Elem()),
		}
	case reflect.Struct:
		return inputSchemaFromArgs(reflect.New(t).Interface())
	default:
		panic(fmt.Sprintf("unsupported schema type: %s", t.String()))
	}
}

func providedSchemaForType(t reflect.Type) (map[string]any, bool) {
	schemaType := reflect.TypeOf((*schemaProvider)(nil)).Elem()

	if t.Implements(schemaType) {
		value := reflect.Zero(t).Interface().(schemaProvider)
		return value.JSONSchema(), true
	}
	if reflect.PointerTo(t).Implements(schemaType) {
		value := reflect.New(t).Interface().(schemaProvider)
		return value.JSONSchema(), true
	}
	return nil, false
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if tag == "" {
		return field.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return field.Name
	}
	return name
}
