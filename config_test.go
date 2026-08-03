package yze_test

import (
	"errors"
	"testing"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gomatic/yze"
)

func TestLoadConfigParsesAnalyzerSettings(t *testing.T) {
	read := func(string) ([]byte, error) {
		return []byte("analyzers:\n  ptrrecv:\n    allow:\n      - pkg.Foo\n      - pkg.Bar\n"), nil
	}

	settings, err := yze.LoadConfig(read, "yze.yaml")

	require.NoError(t, err)
	assert.Equal(t, goyze.Settings{
		"ptrrecv": goyze.AnalyzerSettings{"allow": "pkg.Foo,pkg.Bar"},
	}, settings)
}

func TestErrConfigReportsAnUnreadableFile(t *testing.T) {
	read := func(string) ([]byte, error) { return nil, errs.Const("no file") }

	_, err := yze.LoadConfig(read, "missing.yaml")

	require.Error(t, err)
	assert.True(t, errors.Is(err, yze.ErrConfig))
}

func TestApplySQLConfigRejectsSettingsForBundledAnalyzer(t *testing.T) {
	err := yze.ApplySQLConfig(yze.SQLAnalyzers(), goyze.Settings{
		"keywordcase": goyze.AnalyzerSettings{"style": "upper"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, yze.ErrSQLSetting))
	assert.Contains(t, err.Error(), "keywordcase")
}

func TestApplySQLConfigIgnoresUnknownAnalyzerAndEmptyBlock(t *testing.T) {
	// Mirrors goyze.ApplyConfig: an unknown analyzer name is ignored (a config may
	// target a larger suite than is present), and a block with no settings is a
	// no-op even for a bundled SQL analyzer.
	require.NoError(t, yze.ApplySQLConfig(yze.SQLAnalyzers(), goyze.Settings{
		"someother":   goyze.AnalyzerSettings{"x": "y"},
		"keywordcase": goyze.AnalyzerSettings{},
	}))
}

func TestErrConfigReportsAnUnparseableFile(t *testing.T) {
	read := func(string) ([]byte, error) { return []byte("analyzers: [not a map"), nil }

	_, err := yze.LoadConfig(read, "bad.yaml")

	require.Error(t, err)
	assert.True(t, errors.Is(err, yze.ErrConfig))
}
