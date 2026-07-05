package yze

import (
	"strings"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"gopkg.in/yaml.v3"
)

// Configuration errors.
const (
	// ErrConfig reports a yze configuration file that cannot be read or parsed.
	ErrConfig errs.Const = "cannot load yze config"
	// ErrSQLSetting reports a config setting supplied for a bundled SQL analyzer,
	// none of which defines any settings.
	ErrSQLSetting errs.Const = "SQL analyzer settings are not supported"
)

// fileConfig is the on-disk yze config shape: per-analyzer settings, each a list
// of strings (joined into the analyzer's flag value).
type fileConfig struct {
	Analyzers map[string]map[string][]string `yaml:"analyzers"`
}

// LoadConfig reads and parses a yze config file into per-analyzer settings keyed
// by analyzer name then setting name, ready for go-yze's ApplyConfig. The reader
// is injected so callers control filesystem access.
// ConfigPath is the path to a yze per-analyzer config file.
type ConfigPath string

func LoadConfig(read func(path string) ([]byte, error), path ConfigPath) (goyze.Settings, error) {
	data, err := read(string(path))
	if err != nil {
		return nil, ErrConfig.With(err, "path", string(path))
	}
	var parsed fileConfig
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, ErrConfig.With(err, "path", path)
	}
	return flatten(parsed), nil
}

// ApplySQLConfig checks settings against the suite's SQL analyzers, mirroring
// [goyze.ApplyConfig]'s semantics: an unknown analyzer name is ignored (a config
// may target a larger suite than is present), but a setting supplied for a known
// analyzer must be one it defines — and the SQL analyzers define none, so any
// setting targeting one is [ErrSQLSetting] rather than a silent no-op.
func ApplySQLConfig(analyzers []SQLAnalyzer, settings goyze.Settings) error {
	for _, a := range analyzers {
		for key := range settings[a.Name] {
			return ErrSQLSetting.With(nil, "analyzer", a.Name, "setting", key)
		}
	}
	return nil
}

// flatten joins each setting's list of values into the comma-separated value the
// analyzer flags expect, building go-yze's typed Settings.
func flatten(parsed fileConfig) goyze.Settings {
	settings := make(goyze.Settings, len(parsed.Analyzers))
	for analyzer, values := range parsed.Analyzers {
		analyzerSettings := make(goyze.AnalyzerSettings, len(values))
		for key, list := range values {
			analyzerSettings[goyze.SettingName(key)] = goyze.SettingValue(strings.Join(list, ","))
		}
		settings[goyze.AnalyzerName(analyzer)] = analyzerSettings
	}
	return settings
}
