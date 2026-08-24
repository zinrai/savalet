package main

import (
	"strings"
	"testing"
)

func TestParseConfigValid(t *testing.T) {
	// /bin/sh exists and is executable on any target platform of savalet.
	cfg, err := parseConfig([]byte(`
actions:
  - name: disk-usage
    argv: ["/bin/sh", "{mountpoint}", "{mode}"]
    params:
      mountpoint:
        enum: ["/", "/var"]
      mode:
        enum: ["quick", "full"]
    timeout: 10
`))
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	a, ok := cfg.byName["disk-usage"]
	if !ok {
		t.Fatal("action not registered in byName")
	}
	if a.slots["mountpoint"] != 1 || a.slots["mode"] != 2 {
		t.Errorf("slots = %v, want mountpoint:1 mode:2", a.slots)
	}
	if _, ok := a.Params["mountpoint"].set["/var"]; !ok {
		t.Error("enum set not built")
	}
}

// Every case is one where acceptance would open the authorized set beyond
// what the config declares, or make the reviewed config lie about what
// runs.
func TestParseConfigRejects(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "empty argv",
			yaml: `
actions:
  - name: a
    argv: []
    timeout: 5
`,
			wantErr: "argv must not be empty",
		},
		{
			name: "relative argv0",
			yaml: `
actions:
  - name: a
    argv: ["sh", "-c", "true"]
    timeout: 5
`,
			wantErr: "absolute path",
		},
		{
			name: "partial placeholder",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh", "--unit={service}"]
    params:
      service:
        enum: ["nginx"]
    timeout: 5
`,
			wantErr: "whole element",
		},
		{
			name: "multiple placeholders in one element",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh", "{a}{b}"]
    params:
      a:
        enum: ["x"]
      b:
        enum: ["y"]
    timeout: 5
`,
			wantErr: "whole element",
		},
		{
			name: "literal brace",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh", "-c", "a{b"]
    timeout: 5
`,
			wantErr: "whole element",
		},
		{
			name: "placeholder without param definition",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh", "{service}"]
    timeout: 5
`,
			wantErr: "no param definition",
		},
		{
			name: "param not in argv",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh", "-c", "true"]
    params:
      service:
        enum: ["nginx"]
    timeout: 5
`,
			wantErr: "does not appear in argv",
		},
		{
			name: "duplicate placeholder",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh", "{service}", "{service}"]
    params:
      service:
        enum: ["nginx"]
    timeout: 5
`,
			wantErr: "more than one argv element",
		},
		{
			name: "empty enum value",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh", "{service}"]
    params:
      service:
        enum: ["nginx", ""]
    timeout: 5
`,
			wantErr: "enum value must not be empty",
		},
		{
			name: "dash prefixed enum value",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh", "{service}"]
    params:
      service:
        enum: ["--force"]
    timeout: 5
`,
			wantErr: "must not start with",
		},
		{
			name: "duplicate action name",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh"]
    timeout: 5
  - name: a
    argv: ["/bin/sh"]
    timeout: 5
`,
			wantErr: "duplicate action name",
		},
		{
			name: "unknown action key",
			yaml: `
actions:
  - name: a
    argv: ["/bin/sh"]
    allowed_args: ["-c"]
    timeout: 5
`,
			wantErr: "allowed_args",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig([]byte(tt.yaml))
			if err == nil {
				t.Fatal("parseConfig succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}
