package towerfile

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"minitower/internal/validate"
)

// Towerfile represents a parsed Towerfile.
type Towerfile struct {
	App        App         `toml:"app"`
	Parameters []Parameter `toml:"parameters"`
}

// App holds the [app] section of a Towerfile.
type App struct {
	Name        string   `toml:"name"`
	Script      string   `toml:"script"`
	Source      []string `toml:"source"`
	ImportPaths []string `toml:"import_paths"`
	Timeout     *Timeout `toml:"timeout"`
}

// Timeout holds the [app.timeout] section.
type Timeout struct {
	Seconds int `toml:"seconds"`
}

// Parameter holds a single [[parameters]] entry.
type Parameter struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Type        string `toml:"type"`
	Default     any    `toml:"default"`
}

var (
	ErrMissingName   = errors.New("app.name is required")
	ErrMissingScript = errors.New("app.script is required")
)

var allowedParamTypes = map[string]bool{
	"string":  true,
	"number":  true,
	"integer": true,
	"boolean": true,
}

var allowedScriptExts = map[string]bool{
	".py": true,
	".sh": true,
}

// Parse reads TOML from r and returns a parsed Towerfile.
func Parse(r io.Reader) (*Towerfile, error) {
	var tf Towerfile
	if _, err := toml.NewDecoder(r).Decode(&tf); err != nil {
		return nil, fmt.Errorf("parsing towerfile: %w", err)
	}
	return &tf, nil
}

// Validate checks all Towerfile rules and returns the first error found.
func Validate(tf *Towerfile) error {
	if tf.App.Name == "" {
		return ErrMissingName
	}
	if err := validate.ValidateSlug(tf.App.Name); err != nil {
		return fmt.Errorf("app.name: %w", err)
	}

	if tf.App.Script == "" {
		return ErrMissingScript
	}
	ext := strings.ToLower(filepath.Ext(tf.App.Script))
	if !allowedScriptExts[ext] {
		return fmt.Errorf("app.script must end in .py or .sh, got %q", ext)
	}
	if containsTraversal(tf.App.Script) {
		return fmt.Errorf("app.script must not contain path traversal")
	}

	for _, pattern := range tf.App.Source {
		if containsTraversal(pattern) {
			return fmt.Errorf("source pattern %q must not escape the project root", pattern)
		}
	}

	for _, p := range tf.App.ImportPaths {
		if containsTraversal(p) {
			return fmt.Errorf("import_paths entry %q must not escape the project root", p)
		}
	}

	if tf.App.Timeout != nil && tf.App.Timeout.Seconds < 1 {
		return fmt.Errorf("app.timeout.seconds must be >= 1, got %d", tf.App.Timeout.Seconds)
	}

	seen := make(map[string]bool, len(tf.Parameters))
	for i, param := range tf.Parameters {
		if param.Name == "" {
			return fmt.Errorf("parameters[%d].name is required", i)
		}
		if seen[param.Name] {
			return fmt.Errorf("duplicate parameter name %q", param.Name)
		}
		seen[param.Name] = true

		typ := param.Type
		if typ == "" {
			typ = "string"
		}
		if !allowedParamTypes[typ] {
			return fmt.Errorf("parameters[%d].type must be one of string, number, integer, boolean; got %q", i, typ)
		}

		if param.Default != nil {
			if err := checkDefaultType(param.Default, typ, i); err != nil {
				return err
			}
		}
	}

	return nil
}

// checkDefaultType validates that a TOML-parsed default value is compatible
// with the declared parameter type.
func checkDefaultType(val any, typ string, idx int) error {
	switch typ {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("parameters[%d].default must be a string, got %T", idx, val)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("parameters[%d].default must be a boolean, got %T", idx, val)
		}
	case "integer":
		if _, ok := val.(int64); !ok {
			return fmt.Errorf("parameters[%d].default must be an integer, got %T", idx, val)
		}
	case "number":
		switch val.(type) {
		case float64, int64:
			// both are valid for "number"
		default:
			return fmt.Errorf("parameters[%d].default must be a number, got %T", idx, val)
		}
	}
	return nil
}

// containsTraversal checks if a cleaned path escapes the project root.
func containsTraversal(p string) bool {
	cleaned := filepath.Clean(p)
	if strings.HasPrefix(cleaned, "..") {
		return true
	}
	for _, seg := range strings.Split(cleaned, string(filepath.Separator)) {
		if seg == ".." {
			return true
		}
	}
	return false
}

// ParamsSchemaFromParameters converts []Parameter into a JSON Schema
// map[string]any suitable for storing as params_schema_json.
func ParamsSchemaFromParameters(params []Parameter) map[string]any {
	if len(params) == 0 {
		return nil
	}
	properties := make(map[string]any, len(params))
	for _, p := range params {
		prop := map[string]any{}
		typ := p.Type
		if typ == "" {
			typ = "string"
		}
		prop["type"] = typ
		if p.Description != "" {
			prop["description"] = p.Description
		}
		if p.Default != nil {
			prop["default"] = p.Default
		}
		properties[p.Name] = prop
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
	}
}
