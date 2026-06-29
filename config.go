package yze

import (
	"strings"

	errs "github.com/gomatic/go-error"
	"gopkg.in/yaml.v3"
)

// ErrConfig reports a yze configuration file that cannot be read or parsed.
const ErrConfig errs.Const = "cannot load yze config"

// fileConfig is the on-disk yze config shape: per-analyzer settings, each a list
// of strings (joined into the analyzer's flag value).
type fileConfig struct {
	Analyzers map[string]map[string][]string `yaml:"analyzers"`
}

// LoadConfig reads and parses a yze config file into per-analyzer settings keyed
// by analyzer name then setting name, ready for go-yze's ApplyConfig. The reader
// is injected so callers control filesystem access.
func LoadConfig(read func(path string) ([]byte, error), path string) (map[string]map[string]string, error) {
	data, err := read(path)
	if err != nil {
		return nil, ErrConfig.With(err, "path", path)
	}
	var parsed fileConfig
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, ErrConfig.With(err, "path", path)
	}
	return flatten(parsed), nil
}

// flatten joins each setting's list of values into the comma-separated string the
// analyzer flags expect.
func flatten(parsed fileConfig) map[string]map[string]string {
	settings := make(map[string]map[string]string, len(parsed.Analyzers))
	for analyzer, values := range parsed.Analyzers {
		settings[analyzer] = make(map[string]string, len(values))
		for key, list := range values {
			settings[analyzer][key] = strings.Join(list, ",")
		}
	}
	return settings
}
