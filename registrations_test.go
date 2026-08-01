package yze_test

import (
	"testing"

	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gomatic/yze"
)

func ruleIDs(regs []goyze.Registration) []string {
	ids := make([]string, 0, len(regs))
	for _, r := range regs {
		ids = append(ids, r.RuleID())
	}
	return ids
}

func TestRegistrationsCatalog(t *testing.T) {
	regs := yze.Registrations()

	assert.Equal(
		t,
		[]string{
			"yze/anonstruct",
			"yze/boolname",
			"yze/cliopinion",
			"yze/cliv3",
			"yze/cliversion",
			"yze/ctxfirst",
			"yze/emptyiface",
			"yze/errconst",
			"yze/errlast",
			"yze/errtest",
			"yze/errtested",
			"yze/filesize",
			"yze/globalvar",
			"yze/gotostmt",
			"yze/invariant",
			"yze/jsontag",
			"yze/layout",
			"yze/namedtypes",
			"yze/noinit",
			"yze/nopanic",
			"yze/pkgstd",
			"yze/ptrparam",
			"yze/ptrrecv",
			"yze/slogkv",
			"yze/stdlog",
			"yze/testfile",
			"yze/valuector",
		},
		ruleIDs(regs),
	)
	for _, r := range regs {
		require.NoError(t, r.Validate())
	}
}

func TestFilterByCategorySelectsMatching(t *testing.T) {
	got := yze.Filter(yze.Registrations(), []goyze.Category{"errors"})
	assert.Equal(t, []string{"yze/errconst", "yze/errlast", "yze/errtest", "yze/errtested"}, ruleIDs(got))
}

func TestFilterByMultipleCategories(t *testing.T) {
	got := yze.Filter(yze.Registrations(), []goyze.Category{"errors", "patterns"})
	assert.Equal(t, []string{
		"yze/ctxfirst", "yze/errconst", "yze/errlast", "yze/errtest", "yze/errtested", "yze/gotostmt", "yze/noinit",
		"yze/nopanic",
	}, ruleIDs(got))
}

func TestFilterWithNoConstraintsKeepsAll(t *testing.T) {
	assert.Len(t, yze.Filter(yze.Registrations(), nil), 27)
}

// TestSourceOnlyIsOptedIntoNeverInherited names sourceOnly's claim. The scope
// decides whether a rule may report inside a _test.go file, and both errors are
// real: a rule wrongly marked source-only goes silent on test files (the exact
// failure that once left four analyzers incapable of reporting anything), while
// a rule wrongly left out judges test code by production rules — a test may
// build a bare cli.Command as a flag harness with no Version, and a command
// package's test may import its own domain package however it likes. Judging
// those produced 37 findings across the fleet, every one in a _test.go file.
//
// The assertion is that the default is REPORT-EVERYWHERE: a new analyzer added
// to the suite is unscoped until someone opts it in deliberately.
func TestSourceOnlyIsOptedIntoNeverInherited(t *testing.T) {
	t.Parallel()

	scopes := map[goyze.AnalyzerName]goyze.TestScope{}
	for _, r := range yze.Registrations() {
		scopes[r.Name] = r.TestScope
	}
	require.NotEmpty(t, scopes)

	var scoped, unscoped int
	for name, scope := range scopes {
		switch scope {
		case goyze.TestScopeSourceOnly:
			scoped++
		case goyze.TestScopeAll:
			unscoped++
		default:
			t.Fatalf(
				"%s carries scope %q, which is neither declared value; a third scope must be deliberate",
				name,
				scope,
			)
		}
	}

	assert.Positive(t, scoped, "some rules are production-design rules")
	assert.Positive(t, unscoped, "and the default must remain report-everywhere")

	// A name that is not in the table must come back unscoped — the property
	// that makes the scope opted into rather than inherited.
	assert.NotContains(t, scopes, goyze.AnalyzerName("not-a-real-analyzer"))
}

// TestSourceOnlyNamesOnlyRealAnalyzers guards the table against drift. An entry
// naming an analyzer the suite does not register is dead — and worse, it reads
// as coverage that does not exist: someone scanning the table would conclude a
// rule is scoped when the rule is not in the suite at all, or when it was
// renamed and now reports in test files unnoticed.
func TestSourceOnlyNamesOnlyRealAnalyzers(t *testing.T) {
	t.Parallel()

	registered := map[goyze.AnalyzerName]bool{}
	for _, r := range yze.Registrations() {
		registered[r.Name] = true
	}

	for _, r := range yze.Registrations() {
		if r.TestScope == goyze.TestScopeSourceOnly {
			assert.True(t, registered[r.Name], "%s is scoped but not registered", r.Name)
		}
	}
	assert.Len(t, registered, len(yze.Registrations()), "analyzer names must be unique")
}
