package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

const (
	defaultConfigPath     = "/etc/savalet/savalet.yaml"
	defaultListen         = "unix:/run/savalet/savalet.sock"
	defaultMaxOutputBytes = 1 << 20
	defaultMaxTimeout     = 60
)

var (
	actionNameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	paramNameRe   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	placeholderRe = regexp.MustCompile(`^\{([a-z][a-z0-9_]*)\}$`)
)

type Config struct {
	Listen         string   `yaml:"listen"`
	MaxOutputBytes int      `yaml:"max_output_bytes"`
	MaxTimeout     int      `yaml:"max_timeout"`
	IdentityHeader string   `yaml:"identity_header"`
	Actions        []Action `yaml:"actions"`

	byName     map[string]*Action
	fileSHA256 string
}

type Action struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Argv        []string         `yaml:"argv"`
	Params      map[string]Param `yaml:"params"`
	Timeout     int              `yaml:"timeout"`

	slots map[string]int
}

type Param struct {
	Enum []string `yaml:"enum"`

	set map[string]struct{}
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	cfg.fileSHA256 = hex.EncodeToString(sum[:])
	return cfg, nil
}

func parseConfig(data []byte) (*Config, error) {
	cfg := &Config{
		Listen:         defaultListen,
		MaxOutputBytes: defaultMaxOutputBytes,
		MaxTimeout:     defaultMaxTimeout,
	}
	// Not lenient parsing: this file is the security boundary, and a
	// skipped unknown key would silently authorize a different invocation
	// than the one that was reviewed.
	if err := yaml.UnmarshalWithOptions(data, cfg, yaml.Strict()); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.MaxOutputBytes <= 0 {
		return fmt.Errorf("max_output_bytes must be positive")
	}
	if c.MaxTimeout <= 0 {
		return fmt.Errorf("max_timeout must be positive")
	}
	if len(c.Actions) == 0 {
		return fmt.Errorf("no actions defined")
	}
	c.byName = make(map[string]*Action)
	for i := range c.Actions {
		a := &c.Actions[i]
		if err := a.validate(c.MaxTimeout); err != nil {
			if a.Name != "" {
				return fmt.Errorf("action %q: %w", a.Name, err)
			}
			return fmt.Errorf("actions[%d]: %w", i, err)
		}
		if _, dup := c.byName[a.Name]; dup {
			return fmt.Errorf("duplicate action name: %q", a.Name)
		}
		c.byName[a.Name] = a
	}
	return nil
}

func (a *Action) validate(maxTimeout int) error {
	if !actionNameRe.MatchString(a.Name) {
		return fmt.Errorf("name must match %s: %q", actionNameRe, a.Name)
	}
	if a.Timeout < 1 || a.Timeout > maxTimeout {
		return fmt.Errorf("timeout must be between 1 and %d (max_timeout), got %d", maxTimeout, a.Timeout)
	}
	if len(a.Argv) == 0 {
		return fmt.Errorf("argv must not be empty")
	}
	if !filepath.IsAbs(a.Argv[0]) {
		return fmt.Errorf("argv[0] must be an absolute path: %q", a.Argv[0])
	}
	info, err := os.Stat(a.Argv[0])
	if err != nil {
		return fmt.Errorf("argv[0]: %w", err)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("argv[0] is not executable: %q", a.Argv[0])
	}

	for name, p := range a.Params {
		if !paramNameRe.MatchString(name) {
			return fmt.Errorf("param name must match %s: %q", paramNameRe, name)
		}
		if err := p.validate(); err != nil {
			return fmt.Errorf("param %q: %w", name, err)
		}
		a.Params[name] = p
	}

	a.slots = make(map[string]int)
	for i, elem := range a.Argv {
		if !strings.ContainsAny(elem, "{}") {
			continue
		}
		// Near-miss forms ("--unit={x}", "{a}{b}") are not treated as
		// literals: a typo would silently change what gets executed. The
		// cost is that a literal brace is inexpressible, and a command
		// needing one gets wrapped in a single purpose binary instead.
		m := placeholderRe.FindStringSubmatch(elem)
		if m == nil {
			return fmt.Errorf("argv[%d]: %q is not a valid placeholder (a placeholder must occupy the whole element)", i, elem)
		}
		name := m[1]
		if _, ok := a.Params[name]; !ok {
			return fmt.Errorf("argv[%d]: placeholder %q has no param definition", i, name)
		}
		if _, dup := a.slots[name]; dup {
			return fmt.Errorf("placeholder %q appears in more than one argv element", name)
		}
		a.slots[name] = i
	}
	for name := range a.Params {
		if _, ok := a.slots[name]; !ok {
			return fmt.Errorf("param %q does not appear in argv", name)
		}
	}
	return nil
}

func (p *Param) validate() error {
	if len(p.Enum) == 0 {
		return fmt.Errorf("enum must not be empty")
	}
	p.set = make(map[string]struct{}, len(p.Enum))
	for _, v := range p.Enum {
		if v == "" {
			// An empty value would inject an empty argv element, which
			// changes the meaning of most commands.
			return fmt.Errorf("enum value must not be empty")
		}
		if strings.HasPrefix(v, "-") {
			// Even without a shell, a leading dash can turn a value into
			// an option and change the meaning of the command.
			return fmt.Errorf("enum value must not start with \"-\": %q", v)
		}
		if _, dup := p.set[v]; dup {
			return fmt.Errorf("duplicate enum value: %q", v)
		}
		p.set[v] = struct{}{}
	}
	return nil
}
