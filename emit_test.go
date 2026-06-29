package yze_test

import (
	"bytes"
	"errors"
	"testing"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gomatic/yze"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errs.Const("io fail") }

func reportFixture() goyze.Report {
	return goyze.Report{Diagnostics: []goyze.Diagnostic{
		{
			Tool:     "yze",
			Rule:     "yze/gotostmt",
			Path:     "a.go",
			Line:     3,
			Col:      2,
			Severity: goyze.SeverityError,
			Message:  "goto is not permitted",
		},
	}}
}

func TestEmitStickerJSONRoundTrips(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, yze.Emit(&buf, yze.FormatSticklerJSON, reportFixture()))

	got, err := goyze.UnmarshalReport(buf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, reportFixture(), got)
	assert.Contains(t, buf.String(), `"diagnostics"`)
}

func TestEmitTextFormatsOneLinePerDiagnostic(t *testing.T) {
	var buf bytes.Buffer

	require.NoError(t, yze.Emit(&buf, yze.FormatText, reportFixture()))

	assert.Equal(t, "a.go:3:2: goto is not permitted (yze/gotostmt)\n", buf.String())
}

func TestEmitRejectsUnknownFormat(t *testing.T) {
	err := yze.Emit(&bytes.Buffer{}, "nope", reportFixture())

	require.Error(t, err)
	assert.True(t, errors.Is(err, yze.ErrUnknownFormat))
}

func TestEmitStickerJSONReportsWriterError(t *testing.T) {
	err := yze.Emit(failWriter{}, yze.FormatSticklerJSON, reportFixture())
	require.Error(t, err)
}

func TestEmitTextReportsWriterError(t *testing.T) {
	err := yze.Emit(failWriter{}, yze.FormatText, reportFixture())
	require.Error(t, err)
}
