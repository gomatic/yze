package yze_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
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

// sampleRules is a catalog spanning both languages: the Go registrations above
// plus one SQL-style rule carrying two categories (exercising the category join).
func sampleRules() []yze.Rule {
	return append(yze.GoRules(sampleRegs()), yze.Rule{
		ID:         "yze/keywordcase",
		Name:       "keywordcase",
		Doc:        "bans uppercase SQL keywords",
		URL:        "https://docs.gomatic.dev/yze/keywordcase",
		Categories: []goyze.Category{"sql", "style"},
	})
}

func TestGoRulesMapsRegistrationMetadata(t *testing.T) {
	rules := yze.GoRules(sampleRegs())

	require.Len(t, rules, 2)
	assert.Equal(t, yze.Rule{
		ID:         "yze/errconst",
		Name:       "errconst",
		Doc:        "bans errors.New",
		URL:        "https://docs.gomatic.dev/yze/errconst",
		Categories: []goyze.Category{"errors"},
	}, rules[0])
	assert.Equal(t, "yze/gotostmt", rules[1].ID)
	assert.Empty(t, rules[1].Categories)
}

func TestGoRulesOmitsDocForNilAnalyzer(t *testing.T) {
	rules := yze.GoRules([]goyze.Registration{{Name: "bare"}})

	require.Len(t, rules, 1)
	assert.Equal(t, "yze/bare", rules[0].ID)
	assert.Empty(t, rules[0].Doc)
}

func TestSQLRulesMapsAnalyzerMetadata(t *testing.T) {
	rules := yze.SQLRules(yze.SQLAnalyzers())

	require.Len(t, rules, 1)
	rule := rules[0]
	assert.Equal(t, "yze/keywordcase", rule.ID)
	assert.Equal(t, "keywordcase", rule.Name)
	assert.NotEmpty(t, rule.Doc, "the catalog entry must describe the rule")
	assert.Equal(t, "https://docs.gomatic.dev/yze/keywordcase", rule.URL)
	assert.Equal(t, []goyze.Category{"sql"}, rule.Categories)
}

func TestCatalogRulesMergesBothLanguagesSortedByID(t *testing.T) {
	rules := yze.CatalogRules(yze.Registrations(), yze.SQLAnalyzers())

	assert.Len(t, rules, 31, "the full suite: 30 Go analyzers + 1 SQL analyzer")
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	assert.True(t, slices.IsSorted(ids), "the catalog is in rule-id order")
	assert.Contains(t, ids, "yze/keywordcase")
	assert.Contains(t, ids, "yze/errconst")
}

func TestEmitRulesSARIF(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, yze.EmitRules(&buf, yze.RuleFormatSARIF, sampleRules()))

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
	require.Len(t, driver.Rules, 3)
	assert.Equal(t, "yze/errconst", driver.Rules[0].ID)
	assert.Equal(t, "errconst", driver.Rules[0].Name)
	assert.Equal(t, "bans errors.New", driver.Rules[0].ShortDescription.Text)
	assert.Equal(t, "https://docs.gomatic.dev/yze/errconst", driver.Rules[0].HelpURI)
	assert.Equal(t, []string{"errors"}, driver.Rules[0].Properties.Tags)
	assert.Empty(t, driver.Rules[1].Properties.Tags)
	assert.Equal(t, "yze/keywordcase", driver.Rules[2].ID)
	assert.Equal(t, "keywordcase", driver.Rules[2].Name)
	assert.Equal(t, "bans uppercase SQL keywords", driver.Rules[2].ShortDescription.Text)
	assert.Equal(t, "https://docs.gomatic.dev/yze/keywordcase", driver.Rules[2].HelpURI)
	assert.Equal(t, []string{"sql", "style"}, driver.Rules[2].Properties.Tags)
}

func TestEmitRulesGrit(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, yze.EmitRules(&buf, yze.RuleFormatGrit, sampleRules()))

	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "# yze rule catalog"))
	assert.Contains(t, out, "## `yze/errconst`")
	assert.Contains(t, out, "bans errors.New")
	assert.Contains(t, out, "- docs: https://docs.gomatic.dev/yze/errconst")
	assert.Contains(t, out, "- categories: errors")
	assert.Contains(t, out, "## `yze/gotostmt`")
	assert.Contains(t, out, "- categories: none")
	assert.Contains(t, out, "## `yze/keywordcase`")
	assert.Contains(t, out, "bans uppercase SQL keywords")
	assert.Contains(t, out, "- docs: https://docs.gomatic.dev/yze/keywordcase")
	assert.Contains(t, out, "- categories: sql, style", "multiple categories join with a comma")
}

func TestEmitRulesUnknownFormat(t *testing.T) {
	err := yze.EmitRules(&bytes.Buffer{}, "yaml", sampleRules())
	assert.True(t, errors.Is(err, yze.ErrUnknownRuleFormat))
}

func TestEmitRulesSARIFSurfacesWriteError(t *testing.T) {
	err := yze.EmitRules(&nthFailWriter{ok: 0}, yze.RuleFormatSARIF, sampleRules())
	assert.Error(t, err)
}

func TestEmitRulesGritSurfacesHeaderWriteError(t *testing.T) {
	err := yze.EmitRules(&nthFailWriter{ok: 0}, yze.RuleFormatGrit, sampleRules())
	assert.Error(t, err)
}

func TestEmitRulesGritSurfacesRuleWriteError(t *testing.T) {
	// Header write succeeds; the first rule write fails.
	err := yze.EmitRules(&nthFailWriter{ok: 1}, yze.RuleFormatGrit, sampleRules())
	assert.Error(t, err)
}

// TestRuleFormatCatalogIsBuiltFromMetadataOnly names RuleFormat's claim: the
// exported catalog is built from registration metadata — id, name, doc, url,
// categories — and never from analyzer logic. It matters because the catalog is
// what an editor or a code-scanning platform registers rules from: if building
// it required running or reflecting on analyzers, exporting the catalog could
// fail, hang, or differ between machines for a document that is supposed to be
// a static description of the suite.
//
// The observable form of "metadata only" is determinism and completeness: every
// registered rule appears, with its own id, and repeated exports are identical.
func TestRuleFormatCatalogIsBuiltFromMetadataOnly(t *testing.T) {
	t.Parallel()

	for _, format := range []yze.RuleFormat{yze.RuleFormatSARIF, yze.RuleFormatGrit} {
		t.Run(string(format), func(t *testing.T) {
			t.Parallel()
			var first bytes.Buffer
			require.NoError(t, yze.EmitRules(&first, format, yze.GoRules(yze.Registrations())))

			for _, r := range yze.Registrations() {
				assert.Contains(t, first.String(), r.RuleID(),
					"every registered rule must appear in the %s catalog", format)
			}

			for range 5 {
				var again bytes.Buffer
				require.NoError(t, yze.EmitRules(&again, format, yze.GoRules(yze.Registrations())))
				assert.Equal(t, first.String(), again.String(),
					"a metadata-only catalog is byte-identical on every export")
			}
		})
	}
}

// TestRuleFormatRejectsAnUnknownFormat pins the other half: an unrecognised
// format is a matchable error, not an empty document. Emitting nothing for a
// typo'd --emit-rules value would look like a suite with no rules.
func TestRuleFormatRejectsAnUnknownFormat(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer

	err := yze.EmitRules(&out, yze.RuleFormat("yaml"), yze.GoRules(yze.Registrations()))

	assert.ErrorIs(t, err, yze.ErrUnknownRuleFormat)
	assert.Empty(t, out.String(), "nothing may be written for a format that was refused")
}
