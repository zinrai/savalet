package main

import (
	"reflect"
	"testing"
)

func testAction(t *testing.T) *Action {
	t.Helper()
	cfg, err := parseConfig([]byte(`
actions:
  - name: two-params
    argv: ["/bin/echo", "{first}", "fixed", "{second}"]
    params:
      first:
        enum: ["alpha", "beta"]
      second:
        enum: ["one", "two"]
    timeout: 5
`))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	return cfg.byName["two-params"]
}

func TestValidateParams(t *testing.T) {
	a := testAction(t)
	tests := []struct {
		name    string
		values  map[string]string
		wantErr bool
	}{
		{name: "valid", values: map[string]string{"first": "alpha", "second": "two"}, wantErr: false},
		{name: "unknown key", values: map[string]string{"first": "alpha", "second": "two", "verb": "restart"}, wantErr: true},
		{name: "missing key", values: map[string]string{"first": "alpha"}, wantErr: true},
		{name: "value outside enum", values: map[string]string{"first": "gamma", "second": "two"}, wantErr: true},
		{name: "case sensitive", values: map[string]string{"first": "Alpha", "second": "two"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := a.ValidateParams(tt.values)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateParams(%v) error = %v, wantErr %v", tt.values, err, tt.wantErr)
			}
		})
	}
}

func TestBuildArgv(t *testing.T) {
	a := testAction(t)
	got := a.BuildArgv(map[string]string{"first": "beta", "second": "one"})
	want := []string{"/bin/echo", "beta", "fixed", "one"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BuildArgv = %v, want %v", got, want)
	}
	// The template must not be mutated by a build.
	if a.Argv[1] != "{first}" {
		t.Errorf("action argv mutated: %v", a.Argv)
	}
}
