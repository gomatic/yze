package yze_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"

	"github.com/gomatic/yze"
)

// nthFailWriter writes successfully ok times, then fails every subsequent write —
// so a test can drive both the first-write and a later-write error path.
type nthFailWriter struct {
	written int
	ok      int
}

func (w *nthFailWriter) Write(p []byte) (int, error) {
	if w.written >= w.ok {
		return 0, errs.Const("write boom")
	}
	w.written++
	return len(p), nil
}

func sampleRegs() []goyze.Registration {
	return []goyze.Registration{
		{
			Name:       "errconst",
			URL:        "https://docs.gomatic.dev/yze/errconst",
			Categories: []goyze.Category{"errors"},
			Analyzer:   &analysis.Analyzer{Name: "errconst", Doc: "bans errors.New"},
		},
		{Name: "gotostmt", Analyzer: &analysis.Analyzer{Name: "gotostmt", Doc: "bans goto"}},
	}
}

func TestEmitRulesSARIF(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, yze.EmitRules(&buf, yze.RuleFormatSARIF, sampleRegs()))

	var log struct {
		Schema  string `json:"$schema"`
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name  string `json:"name"`
					Rules []struct {
						ID               string `json:"id"`
						Name             string `json:"name"`
						ShortDescription struct {
							Text string `json:"text"`
						} `json:"shortDescription"`
						HelpURI    string `json:"helpUri"`
						Properties struct {
							Tags []string `json:"tags"`
						} `json:"properties"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &log))
	assert.Equal(t, "2.1.0", log.Version)
	assert.Contains(t, log.Schema, "sarif-2.1.0")
	require.Len(t, log.Runs, 1)
	driver := log.Runs[0].Tool.Driver
	assert.Equal(t, "yze", driver.Name)
	require.Len(t, driver.Rules, 2)
	assert.Equal(t, "yze/errconst", driver.Rules[0].ID)
	assert.Equal(t, "errconst", driver.Rules[0].Name)
	assert.Equal(t, "bans errors.New", driver.Rules[0].ShortDescription.Text)
	assert.Equal(t, "https://docs.gomatic.dev/yze/errconst", driver.Rules[0].HelpURI)
	assert.Equal(t, []string{"errors"}, driver.Rules[0].Properties.Tags)
	assert.Empty(t, driver.Rules[1].Properties.Tags)
}

func TestEmitRulesSARIFOmitsDocForNilAnalyzer(t *testing.T) {
	var buf bytes.Buffer
	regs := []goyze.Registration{{Name: "bare"}}
	require.NoError(t, yze.EmitRules(&buf, yze.RuleFormatSARIF, regs))
	assert.Contains(t, buf.String(), `"text": ""`)
}

func TestEmitRulesGrit(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, yze.EmitRules(&buf, yze.RuleFormatGrit, sampleRegs()))

	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "# yze rule catalog"))
	assert.Contains(t, out, "## `yze/errconst`")
	assert.Contains(t, out, "bans errors.New")
	assert.Contains(t, out, "- docs: https://docs.gomatic.dev/yze/errconst")
	assert.Contains(t, out, "- categories: errors")
	assert.Contains(t, out, "## `yze/gotostmt`")
	assert.Contains(t, out, "- categories: none")
}

func TestEmitRulesUnknownFormat(t *testing.T) {
	err := yze.EmitRules(&bytes.Buffer{}, "yaml", sampleRegs())
	assert.True(t, errors.Is(err, yze.ErrUnknownRuleFormat))
}

func TestEmitRulesSARIFSurfacesWriteError(t *testing.T) {
	err := yze.EmitRules(&nthFailWriter{ok: 0}, yze.RuleFormatSARIF, sampleRegs())
	assert.Error(t, err)
}

func TestEmitRulesGritSurfacesHeaderWriteError(t *testing.T) {
	err := yze.EmitRules(&nthFailWriter{ok: 0}, yze.RuleFormatGrit, sampleRegs())
	assert.Error(t, err)
}

func TestEmitRulesGritSurfacesRuleWriteError(t *testing.T) {
	// Header write succeeds; the first rule write fails.
	err := yze.EmitRules(&nthFailWriter{ok: 1}, yze.RuleFormatGrit, sampleRegs())
	assert.Error(t, err)
}
