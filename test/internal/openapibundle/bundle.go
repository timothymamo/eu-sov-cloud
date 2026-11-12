package openapibundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Path is where the bundle of api/openapi.json is committed, relative to the
// repository root: test/openapibundle writes it, and the release publishes
// that file as shift2eu-server_openapi.json (.goreleaser.yaml's glob is
// "./" + Path), so the two cannot name different files.
const Path = "api/openapi.bundled.json"

// schemasPointer is where the bundle adds the schemas it reads.
const schemasPointer = "#/components/schemas/"

// version is the last kebab part of a schema's base name: v and a number.
var version = regexp.MustCompile(`^v[0-9]+$`)

// kebabPart is every other kebab part of a schema's base name: a lowercase
// ASCII letter, then lowercase ASCII letters and digits. Title-cased, the
// parts make a name that is a valid OpenAPI component key and holds no ~ or
// /, which a JSON pointer would have to escape.
var kebabPart = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// Bundle returns the OpenAPI document at docPath in fsys with every schema it
// refers to by a path added to components.schemas, and every ref to such a
// schema a pointer into the document, as the package documentation says.
func Bundle(fsys fs.FS, docPath string) ([]byte, error) {
	doc, err := readObject(fsys, docPath)
	if err != nil {
		return nil, err
	}

	// files maps each file the document refers to, by its path in fsys, to
	// its name in components.schemas.
	files := map[string]string{}

	var walkErr error

	walk(doc, func(ref string) string {
		if walkErr != nil || strings.HasPrefix(ref, "#") {
			return ref
		}

		bundled, err := bundleRef(docPath, ref, files)
		if err != nil {
			walkErr = err

			return ref
		}

		return bundled
	})

	if walkErr != nil {
		return nil, walkErr
	}

	schemas, err := componentSchemas(doc, docPath)
	if err != nil {
		return nil, err
	}

	for _, file := range sortedKeys(files) {
		name := files[file]

		if _, taken := schemas[name]; taken {
			return nil, fmt.Errorf("%s: components.schemas already holds %s, the name of %s", docPath, name, file)
		}

		schema, err := readSchema(fsys, file, name)
		if err != nil {
			return nil, err
		}

		schemas[name] = schema
	}

	// An Encoder, not MarshalIndent, so that <, > and & stay as they are
	// written (Bearer <token>) instead of \u003c, \u003e and \u0026. Encode
	// ends the document with its one newline.
	var out bytes.Buffer

	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")

	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("write the bundle of %s: %w", docPath, err)
	}

	return out.Bytes(), nil
}

// bundleRef returns the pointer into the bundle that replaces ref, a $ref of
// the document at docPath to a file, and records the file in files.
func bundleRef(docPath, ref string, files map[string]string) (string, error) {
	file, fragment, _ := strings.Cut(ref, "#")
	if fragment != "" && !strings.HasPrefix(fragment, "/") {
		return "", fmt.Errorf("%s: the ref %q has a fragment that is not a JSON pointer", docPath, ref)
	}

	target := path.Join(path.Dir(docPath), file)
	if !fs.ValidPath(target) {
		return "", fmt.Errorf("%s: the ref %q resolves to %s, outside the root", docPath, ref, target)
	}

	name, err := schemaName(target)
	if err != nil {
		return "", fmt.Errorf("%s: the ref %q: %w", docPath, ref, err)
	}

	for other, otherName := range files {
		if otherName == name && other != target {
			return "", fmt.Errorf("%s: %s and %s are both named %s", docPath, other, target, name)
		}
	}

	files[target] = name

	return schemasPointer + name + fragment, nil
}

// schemaName returns the name the schema file has in components.schemas: the
// base name with the kebab parts title-cased, Report inserted before the
// version and the -vN suffix written VN (assessment-v1.schema.json is
// AssessmentReportV1).
func schemaName(file string) (string, error) {
	base := path.Base(file)

	stem, ok := strings.CutSuffix(base, ".schema.json")
	if !ok {
		stem, ok = strings.CutSuffix(base, ".json")
	}

	if !ok {
		return "", fmt.Errorf("%s is not a .json file", file)
	}

	parts := strings.Split(stem, "-")
	if len(parts) < 2 || !version.MatchString(parts[len(parts)-1]) {
		return "", fmt.Errorf("%s is not named <name>-vN.schema.json", file)
	}

	var name strings.Builder

	for _, part := range parts[:len(parts)-1] {
		if !kebabPart.MatchString(part) {
			return "", fmt.Errorf("%s has a kebab part %q that is not a lowercase letter followed by lowercase letters and digits", file, part)
		}

		name.WriteString(strings.ToUpper(part[:1]))
		name.WriteString(part[1:])
	}

	name.WriteString("Report")
	name.WriteString("V" + strings.TrimPrefix(parts[len(parts)-1], "v"))

	return name.String(), nil
}

// readSchema reads the schema file to be added as components.schemas.<name>:
// its $id and $schema removed and its own pointers rewritten to absolute ones
// into the bundle.
func readSchema(fsys fs.FS, file, name string) (map[string]any, error) {
	schema, err := readObject(fsys, file)
	if err != nil {
		return nil, err
	}

	delete(schema, "$id")
	delete(schema, "$schema")

	base := schemasPointer + name

	var walkErr error

	walk(schema, func(ref string) string {
		switch {
		case ref == "#":
			return base
		case strings.HasPrefix(ref, "#/"):
			return base + ref[1:]
		default:
			if walkErr == nil {
				walkErr = fmt.Errorf("%s: the ref %q is not a pointer into the schema itself, want # or a #/ pointer", file, ref)
			}

			return ref
		}
	})

	if walkErr != nil {
		return nil, walkErr
	}

	return schema, nil
}

// componentSchemas returns components.schemas of doc, creating both when the
// document has neither.
func componentSchemas(doc map[string]any, docPath string) (map[string]any, error) {
	components, err := childObject(doc, "components", docPath)
	if err != nil {
		return nil, err
	}

	return childObject(components, "schemas", docPath+": components")
}

// childObject returns the object parent holds under key, adding an empty one
// when it holds none.
func childObject(parent map[string]any, key, where string) (map[string]any, error) {
	v, ok := parent[key]
	if !ok {
		child := map[string]any{}
		parent[key] = child

		return child, nil
	}

	child, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: %s is not an object", where, key)
	}

	return child, nil
}

// readObject reads the file at name in fsys as one JSON object, keeping every
// number as it is written.
func readObject(fsys fs.FS, name string) (map[string]any, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	if obj == nil {
		return nil, fmt.Errorf("read %s: not a JSON object", name)
	}

	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read %s: more than one JSON value", name)
	}

	return obj, nil
}

// walk calls rewrite on every string $ref v holds and stores what it returns
// in its place. Keys are walked sorted, so the first error is the same every
// run.
func walk(v any, rewrite func(ref string) string) {
	switch v := v.(type) {
	case map[string]any:
		for _, key := range sortedKeys(v) {
			if ref, ok := v[key].(string); ok && key == "$ref" {
				v[key] = rewrite(ref)

				continue
			}

			walk(v[key], rewrite)
		}
	case []any:
		for _, item := range v {
			walk(item, rewrite)
		}
	}
}

// sortedKeys returns the keys of m, sorted.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}
