// Package contract turns the Go types the API actually serves into the
// TypeScript types and Zod schemas its clients consume.
//
// This is the resolution of B-2 (ADR-007). Before it, one error taxonomy
// existed three times by hand — Go constants, a TypeScript union, a Zod enum —
// and keeping them equal was a matter of remembering to. Here the Go type is
// read by reflection, so the emitted TypeScript cannot describe a shape the
// server does not serve. Drift is not detected; it is unrepresentable.
//
// The registry is deliberately small. It understands the shapes this platform
// puts on the wire and refuses anything else, loudly, at generation time. It
// is not a general-purpose IDL, and it should not grow into one.
package contract

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// Registry collects the types that form the wire contract.
type Registry struct {
	enums   []enum
	structs []structType
	aliases []alias
	fields  map[string]scalar
	scalars map[reflect.Type]scalar
	byType  map[reflect.Type]string
}

type alias struct {
	tsName string
	zod    string
}

type enum struct {
	tsName    string
	constName string
	values    []string
}

type structType struct {
	tsName string
	goType reflect.Type
}

type scalar struct {
	ts  string
	zod string
}

func New() *Registry {
	r := &Registry{
		fields:  map[string]scalar{},
		scalars: map[reflect.Type]scalar{},
		byType:  map[reflect.Type]string{},
	}
	// A timestamp crosses the wire as an RFC 3339 string in UTC (documents 13,
	// 26 and 150). Clients receive a string and parse it deliberately, rather
	// than a number whose units they must guess.
	r.Scalar(reflect.TypeOf(time.Time{}), "string", "z.string().datetime({ offset: true })")
	return r
}

// Scalar maps a Go type to a TypeScript type and Zod expression directly,
// for types whose structure should not be walked.
func (r *Registry) Scalar(goType reflect.Type, ts, zod string) {
	r.scalars[goType] = scalar{ts: ts, zod: zod}
}

// Field overrides the mapping for one field of one registered struct, keyed by
// the struct's TypeScript name and the field's JSON name. It exists for
// constraints that live on a field rather than on a type — the bound on a
// money amount, not the shape of one.
func (r *Registry) Field(structName, jsonName, ts, zod string) {
	r.fields[structName+"."+jsonName] = scalar{ts: ts, zod: zod}
}

// Enum registers a string-backed Go type as a closed set of values. The values
// are supplied rather than discovered because Go constants are not enumerable
// at runtime; the accompanying test asserts the list matches the constants.
//
// constName is given rather than derived: mechanical pluralisation produces
// names like HEALTH_STATUSS, and a contract clients read should not carry the
// scars of the tool that wrote it.
func (r *Registry) Enum(tsName, constName string, goType reflect.Type, values ...string) {
	r.enums = append(r.enums, enum{tsName: tsName, constName: constName, values: values})
	r.byType[goType] = tsName
}

// Pattern registers a string-backed Go type whose values are open-ended but
// constrained by a regular expression — an event name, not a status. It is a
// plain string to TypeScript and a validated string to Zod.
func (r *Registry) Pattern(tsName string, goType reflect.Type, expression string) {
	r.byType[goType] = tsName
	r.aliases = append(r.aliases, alias{tsName: tsName, zod: fmt.Sprintf("z.string().regex(/%s/)", expression)})
}

// Struct registers a Go struct under the name clients will know it by. The
// sample is used only for its type.
func (r *Registry) Struct(tsName string, sample any) {
	goType := reflect.TypeOf(sample)
	r.structs = append(r.structs, structType{tsName: tsName, goType: goType})
	r.byType[goType] = tsName
}

type field struct {
	name     string
	optional bool
	goType   reflect.Type
}

// fieldsOf reads a struct's JSON representation: the names clients see and
// which of them may be absent.
func fieldsOf(goType reflect.Type) ([]field, error) {
	if goType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("contract: %s is not a struct", goType)
	}
	var out []field
	for i := 0; i < goType.NumField(); i++ {
		f := goType.Field(i)
		if f.PkgPath != "" {
			continue // unexported; never serialised
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		out = append(out, field{
			name: name,
			// omitempty is the only signal Go gives that a field may be
			// absent, so it is what "optional" means on the client.
			optional: strings.Contains(opts, "omitempty"),
			goType:   f.Type,
		})
	}
	return out, nil
}

func (r *Registry) tsType(goType reflect.Type) (string, error) {
	if s, ok := r.scalars[goType]; ok {
		return s.ts, nil
	}
	if name, ok := r.byType[goType]; ok {
		return name, nil
	}
	switch goType.Kind() {
	case reflect.String:
		return "string", nil
	case reflect.Bool:
		return "boolean", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number", nil
	case reflect.Slice, reflect.Array:
		elem, err := r.tsType(goType.Elem())
		if err != nil {
			return "", err
		}
		return elem + "[]", nil
	case reflect.Map:
		key, err := r.tsType(goType.Key())
		if err != nil {
			return "", err
		}
		value, err := r.tsType(goType.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Record<%s, %s>", key, value), nil
	case reflect.Pointer:
		return r.tsType(goType.Elem())
	case reflect.Interface:
		if goType.NumMethod() == 0 {
			return "unknown", nil
		}
	}
	return "", fmt.Errorf("contract: no TypeScript mapping for %s — register it before generating", goType)
}

func (r *Registry) zodType(goType reflect.Type) (string, error) {
	if s, ok := r.scalars[goType]; ok {
		return s.zod, nil
	}
	if name, ok := r.byType[goType]; ok {
		return schemaName(name), nil
	}
	switch goType.Kind() {
	case reflect.String:
		return "z.string()", nil
	case reflect.Bool:
		return "z.boolean()", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "z.number().int()", nil
	case reflect.Float32, reflect.Float64:
		return "z.number()", nil
	case reflect.Slice, reflect.Array:
		elem, err := r.zodType(goType.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("z.array(%s)", elem), nil
	case reflect.Map:
		key, err := r.zodType(goType.Key())
		if err != nil {
			return "", err
		}
		value, err := r.zodType(goType.Elem())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("z.record(%s, %s)", key, value), nil
	case reflect.Pointer:
		return r.zodType(goType.Elem())
	case reflect.Interface:
		if goType.NumMethod() == 0 {
			return "z.unknown()", nil
		}
	}
	return "", fmt.Errorf("contract: no Zod mapping for %s — register it before generating", goType)
}

func schemaName(tsName string) string {
	return strings.ToLower(tsName[:1]) + tsName[1:] + "Schema"
}

const banner = `// Code generated by services/api/cmd/contractgen. DO NOT EDIT.
//
// The Go types in services/api are authoritative for every shape below
// (ADR-007). To change one, change the Go type and run "make contracts".
`

// EmitTypeScript writes the type declarations for @platform/types.
func (r *Registry) EmitTypeScript() (string, error) {
	var b strings.Builder
	b.WriteString(banner)

	for _, e := range r.enums {
		values := make([]string, len(e.values))
		for i, v := range e.values {
			values[i] = fmt.Sprintf("  '%s',", v)
		}
		fmt.Fprintf(&b, "\nexport const %s = [\n%s\n] as const;\n", e.constName, strings.Join(values, "\n"))
		fmt.Fprintf(&b, "\nexport type %s = (typeof %s)[number];\n", e.tsName, e.constName)
	}

	for _, a := range r.aliases {
		fmt.Fprintf(&b, "\nexport type %s = string;\n", a.tsName)
	}

	for _, s := range r.structs {
		fields, err := fieldsOf(s.goType)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\nexport interface %s {\n", s.tsName)
		for _, f := range fields {
			ts := ""
			if override, ok := r.fields[s.tsName+"."+f.name]; ok && override.ts != "" {
				ts = override.ts
			} else {
				resolved, err := r.tsType(f.goType)
				if err != nil {
					return "", fmt.Errorf("%s.%s: %w", s.tsName, f.name, err)
				}
				ts = resolved
			}
			if f.optional {
				// `| undefined` is required by exactOptionalPropertyTypes,
				// which the workspace tsconfig enables.
				fmt.Fprintf(&b, "  %s?: %s | undefined;\n", f.name, ts)
			} else {
				fmt.Fprintf(&b, "  %s: %s;\n", f.name, ts)
			}
		}
		b.WriteString("}\n")
	}
	return b.String(), nil
}

// EmitZod writes the runtime schemas for @platform/validation.
func (r *Registry) EmitZod() (string, error) {
	var b strings.Builder
	b.WriteString(banner)
	b.WriteString("\nimport { z } from 'zod';\n")

	for _, e := range r.enums {
		values := make([]string, len(e.values))
		for i, v := range e.values {
			values[i] = fmt.Sprintf("'%s'", v)
		}
		fmt.Fprintf(&b, "\nexport const %s = z.enum([%s]);\n", schemaName(e.tsName), strings.Join(values, ", "))
	}

	for _, a := range r.aliases {
		fmt.Fprintf(&b, "\nexport const %s = %s;\n", schemaName(a.tsName), a.zod)
	}

	for _, s := range r.structs {
		fields, err := fieldsOf(s.goType)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\nexport const %s = z.object({\n", schemaName(s.tsName))
		for _, f := range fields {
			zod := ""
			if override, ok := r.fields[s.tsName+"."+f.name]; ok && override.zod != "" {
				zod = override.zod
			} else {
				resolved, err := r.zodType(f.goType)
				if err != nil {
					return "", fmt.Errorf("%s.%s: %w", s.tsName, f.name, err)
				}
				zod = resolved
			}
			if f.optional {
				zod += ".optional()"
			}
			fmt.Fprintf(&b, "  %s: %s,\n", f.name, zod)
		}
		b.WriteString("});\n")
	}
	return b.String(), nil
}

// Names lists every registered TypeScript name, for tests that assert the
// registry covers the contract.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.enums)+len(r.structs)+len(r.aliases))
	for _, e := range r.enums {
		out = append(out, e.tsName)
	}
	for _, a := range r.aliases {
		out = append(out, a.tsName)
	}
	for _, s := range r.structs {
		out = append(out, s.tsName)
	}
	sort.Strings(out)
	return out
}
