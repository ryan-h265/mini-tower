package towerfile

import (
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	input := `
[app]
name = "my-etl-pipeline"
script = "./pipeline.py"
source = ["./**/*.py", "./requirements.txt"]
import_paths = ["./lib"]

[app.timeout]
seconds = 120

[[parameters]]
name = "region"
description = "AWS region"
type = "string"
default = "us-east-1"

[[parameters]]
name = "batch_size"
description = "Number of records"
type = "integer"
default = 100
`
	tf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if tf.App.Name != "my-etl-pipeline" {
		t.Errorf("Name = %q, want %q", tf.App.Name, "my-etl-pipeline")
	}
	if tf.App.Script != "./pipeline.py" {
		t.Errorf("Script = %q, want %q", tf.App.Script, "./pipeline.py")
	}
	if len(tf.App.Source) != 2 {
		t.Fatalf("Source len = %d, want 2", len(tf.App.Source))
	}
	if tf.App.Source[0] != "./**/*.py" {
		t.Errorf("Source[0] = %q, want %q", tf.App.Source[0], "./**/*.py")
	}
	if len(tf.App.ImportPaths) != 1 || tf.App.ImportPaths[0] != "./lib" {
		t.Errorf("ImportPaths = %v, want [./lib]", tf.App.ImportPaths)
	}
	if tf.App.Timeout == nil || tf.App.Timeout.Seconds != 120 {
		t.Errorf("Timeout = %v, want 120s", tf.App.Timeout)
	}
	if len(tf.Parameters) != 2 {
		t.Fatalf("Parameters len = %d, want 2", len(tf.Parameters))
	}
	if tf.Parameters[0].Name != "region" {
		t.Errorf("Parameters[0].Name = %q, want %q", tf.Parameters[0].Name, "region")
	}
	if tf.Parameters[0].Default != "us-east-1" {
		t.Errorf("Parameters[0].Default = %v, want %q", tf.Parameters[0].Default, "us-east-1")
	}
	if tf.Parameters[1].Name != "batch_size" {
		t.Errorf("Parameters[1].Name = %q, want %q", tf.Parameters[1].Name, "batch_size")
	}
	// TOML parses integers as int64.
	if tf.Parameters[1].Default != int64(100) {
		t.Errorf("Parameters[1].Default = %v (%T), want int64(100)", tf.Parameters[1].Default, tf.Parameters[1].Default)
	}
}

func TestParseMinimal(t *testing.T) {
	input := `
[app]
name = "simple-app"
script = "main.py"
`
	tf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if tf.App.Timeout != nil {
		t.Errorf("Timeout should be nil, got %v", tf.App.Timeout)
	}
	if len(tf.App.Source) != 0 {
		t.Errorf("Source should be empty, got %v", tf.App.Source)
	}
	if len(tf.Parameters) != 0 {
		t.Errorf("Parameters should be empty, got %v", tf.Parameters)
	}
}

func TestParseShellEntrypoint(t *testing.T) {
	input := `
[app]
name = "shell-app"
script = "run.sh"
`
	tf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if err := Validate(tf); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestParseInvalidTOML(t *testing.T) {
	input := `this is not valid toml [[[`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("Parse() should fail on invalid TOML")
	}
}

func TestValidateMissingName(t *testing.T) {
	tf := &Towerfile{App: App{Script: "main.py"}}
	err := Validate(tf)
	if err != ErrMissingName {
		t.Errorf("Validate() = %v, want ErrMissingName", err)
	}
}

func TestValidateMissingScript(t *testing.T) {
	tf := &Towerfile{App: App{Name: "my-app"}}
	err := Validate(tf)
	if err != ErrMissingScript {
		t.Errorf("Validate() = %v, want ErrMissingScript", err)
	}
}

func TestValidateInvalidSlug(t *testing.T) {
	tests := []struct {
		name string
		slug string
	}{
		{"too short", "ab"},
		{"starts with digit", "1app"},
		{"uppercase", "MyApp"},
		{"reserved", "admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tf := &Towerfile{App: App{Name: tt.slug, Script: "main.py"}}
			if err := Validate(tf); err == nil {
				t.Errorf("Validate() should reject slug %q", tt.slug)
			}
		})
	}
}

func TestValidateInvalidScriptExtension(t *testing.T) {
	tf := &Towerfile{App: App{Name: "my-app", Script: "main.rb"}}
	err := Validate(tf)
	if err == nil {
		t.Fatal("Validate() should reject .rb extension")
	}
	if !strings.Contains(err.Error(), ".py or .sh") {
		t.Errorf("error should mention allowed extensions, got: %v", err)
	}
}

func TestValidateScriptTraversal(t *testing.T) {
	tf := &Towerfile{App: App{Name: "my-app", Script: "../escape.py"}}
	err := Validate(tf)
	if err == nil {
		t.Fatal("Validate() should reject path traversal in script")
	}
}

func TestValidateSourceTraversal(t *testing.T) {
	tf := &Towerfile{App: App{
		Name:   "my-app",
		Script: "main.py",
		Source:  []string{"../../etc/passwd"},
	}}
	err := Validate(tf)
	if err == nil {
		t.Fatal("Validate() should reject path traversal in source")
	}
}

func TestValidateImportPathsTraversal(t *testing.T) {
	tf := &Towerfile{App: App{
		Name:        "my-app",
		Script:      "main.py",
		ImportPaths: []string{"../outside"},
	}}
	err := Validate(tf)
	if err == nil {
		t.Fatal("Validate() should reject path traversal in import_paths")
	}
}

func TestValidateTimeoutZero(t *testing.T) {
	tf := &Towerfile{App: App{
		Name:    "my-app",
		Script:  "main.py",
		Timeout: &Timeout{Seconds: 0},
	}}
	err := Validate(tf)
	if err == nil {
		t.Fatal("Validate() should reject timeout of 0")
	}
}

func TestValidateTimeoutNegative(t *testing.T) {
	tf := &Towerfile{App: App{
		Name:    "my-app",
		Script:  "main.py",
		Timeout: &Timeout{Seconds: -5},
	}}
	err := Validate(tf)
	if err == nil {
		t.Fatal("Validate() should reject negative timeout")
	}
}

func TestValidateDuplicateParameterNames(t *testing.T) {
	tf := &Towerfile{
		App: App{Name: "my-app", Script: "main.py"},
		Parameters: []Parameter{
			{Name: "region"},
			{Name: "region"},
		},
	}
	err := Validate(tf)
	if err == nil {
		t.Fatal("Validate() should reject duplicate parameter names")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate, got: %v", err)
	}
}

func TestValidateEmptyParameterName(t *testing.T) {
	tf := &Towerfile{
		App:        App{Name: "my-app", Script: "main.py"},
		Parameters: []Parameter{{Name: ""}},
	}
	err := Validate(tf)
	if err == nil {
		t.Fatal("Validate() should reject empty parameter name")
	}
}

func TestValidateInvalidParameterType(t *testing.T) {
	tf := &Towerfile{
		App:        App{Name: "my-app", Script: "main.py"},
		Parameters: []Parameter{{Name: "x", Type: "float"}},
	}
	err := Validate(tf)
	if err == nil {
		t.Fatal("Validate() should reject invalid parameter type")
	}
}

func TestValidateDefaultTypeMismatch(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		def     any
		wantErr bool
	}{
		{"string match", "string", "hello", false},
		{"string mismatch", "string", int64(42), true},
		{"integer match", "integer", int64(42), false},
		{"integer mismatch", "integer", "hello", true},
		{"number int64", "number", int64(42), false},
		{"number float64", "number", float64(3.14), false},
		{"number mismatch", "number", "hello", true},
		{"boolean match", "boolean", true, false},
		{"boolean mismatch", "boolean", int64(1), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tf := &Towerfile{
				App:        App{Name: "my-app", Script: "main.py"},
				Parameters: []Parameter{{Name: "x", Type: tt.typ, Default: tt.def}},
			}
			err := Validate(tf)
			if tt.wantErr && err == nil {
				t.Fatal("Validate() should have returned an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateDefaultTypeOmitted(t *testing.T) {
	// When type is omitted it defaults to "string", so a string default is valid.
	tf := &Towerfile{
		App:        App{Name: "my-app", Script: "main.py"},
		Parameters: []Parameter{{Name: "x", Default: "hello"}},
	}
	if err := Validate(tf); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidateValid(t *testing.T) {
	tf := &Towerfile{
		App: App{
			Name:        "my-app",
			Script:      "main.py",
			Source:      []string{"./**/*.py"},
			ImportPaths: []string{"./lib"},
			Timeout:     &Timeout{Seconds: 60},
		},
		Parameters: []Parameter{
			{Name: "region", Type: "string", Default: "us-east-1"},
			{Name: "count", Type: "integer", Default: int64(10)},
		},
	}
	if err := Validate(tf); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestParamsSchemaFromParameters(t *testing.T) {
	params := []Parameter{
		{Name: "region", Description: "AWS region", Type: "string", Default: "us-east-1"},
		{Name: "count", Type: "integer"},
		{Name: "verbose", Type: "boolean", Default: true},
	}

	schema := ParamsSchemaFromParameters(params)
	if schema == nil {
		t.Fatal("schema should not be nil")
	}
	if schema["type"] != "object" {
		t.Errorf("schema.type = %v, want object", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema.properties has wrong type: %T", schema["properties"])
	}
	if len(props) != 3 {
		t.Fatalf("properties len = %d, want 3", len(props))
	}

	region, ok := props["region"].(map[string]any)
	if !ok {
		t.Fatalf("region has wrong type: %T", props["region"])
	}
	if region["type"] != "string" {
		t.Errorf("region.type = %v, want string", region["type"])
	}
	if region["description"] != "AWS region" {
		t.Errorf("region.description = %v, want AWS region", region["description"])
	}
	if region["default"] != "us-east-1" {
		t.Errorf("region.default = %v, want us-east-1", region["default"])
	}

	count, ok := props["count"].(map[string]any)
	if !ok {
		t.Fatalf("count has wrong type: %T", props["count"])
	}
	if count["type"] != "integer" {
		t.Errorf("count.type = %v, want integer", count["type"])
	}
	if _, exists := count["description"]; exists {
		t.Error("count should not have description")
	}
	if _, exists := count["default"]; exists {
		t.Error("count should not have default")
	}
}

func TestParamsSchemaFromParametersEmpty(t *testing.T) {
	schema := ParamsSchemaFromParameters(nil)
	if schema != nil {
		t.Errorf("schema should be nil for empty params, got %v", schema)
	}
}

func TestParamsSchemaDefaultType(t *testing.T) {
	// When type is omitted, it defaults to "string".
	params := []Parameter{{Name: "foo"}}
	schema := ParamsSchemaFromParameters(params)
	props := schema["properties"].(map[string]any)
	foo := props["foo"].(map[string]any)
	if foo["type"] != "string" {
		t.Errorf("foo.type = %v, want string", foo["type"])
	}
}
