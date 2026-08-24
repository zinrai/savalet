package main

import "fmt"

// An unknown key is an error rather than ignored: silently dropping it
// would hide a client bug behind a successful execution.
func (a *Action) ValidateParams(values map[string]string) error {
	for name := range values {
		if _, ok := a.Params[name]; !ok {
			return fmt.Errorf("unknown parameter: %s", capString(name))
		}
	}
	for name, p := range a.Params {
		v, ok := values[name]
		if !ok {
			return fmt.Errorf("missing parameter: %s", name)
		}
		if _, ok := p.set[v]; !ok {
			return fmt.Errorf("invalid value for parameter %s: %q", name, capString(v))
		}
	}
	return nil
}

// Values replace whole argv elements rather than being concatenated, so
// no value can alter the structure of the command.
func (a *Action) BuildArgv(values map[string]string) []string {
	argv := make([]string, len(a.Argv))
	copy(argv, a.Argv)
	for name, idx := range a.slots {
		argv[idx] = values[name]
	}
	return argv
}
