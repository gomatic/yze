package yze_test

import (
	"errors"
	"testing"

	errs "github.com/gomatic/go-error"
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
	assert.Equal(t, "pkg.Foo,pkg.Bar", settings["ptrrecv"]["allow"])
}

func TestLoadConfigReportsReadError(t *testing.T) {
	read := func(string) ([]byte, error) { return nil, errs.Const("no file") }

	_, err := yze.LoadConfig(read, "missing.yaml")

	require.Error(t, err)
	assert.True(t, errors.Is(err, yze.ErrConfig))
}

func TestLoadConfigReportsParseError(t *testing.T) {
	read := func(string) ([]byte, error) { return []byte("analyzers: [not a map"), nil }

	_, err := yze.LoadConfig(read, "bad.yaml")

	require.Error(t, err)
	assert.True(t, errors.Is(err, yze.ErrConfig))
}
