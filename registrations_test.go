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
			"yze/ctxfirst",
			"yze/emptyiface",
			"yze/errconst",
			"yze/errlast",
			"yze/gotostmt",
			"yze/layout",
			"yze/namedtypes",
			"yze/pkgstd",
			"yze/ptrparam",
			"yze/ptrrecv",
			"yze/stdlog",
			"yze/testfile",
		},
		ruleIDs(regs),
	)
	for _, r := range regs {
		require.NoError(t, r.Validate())
	}
}

func TestFilterByCategorySelectsMatching(t *testing.T) {
	got := yze.Filter(yze.Registrations(), []goyze.Category{"errors"})
	assert.Equal(t, []string{"yze/errconst", "yze/errlast"}, ruleIDs(got))
}

func TestFilterByMultipleCategories(t *testing.T) {
	got := yze.Filter(yze.Registrations(), []goyze.Category{"errors", "patterns"})
	assert.Equal(t, []string{"yze/ctxfirst", "yze/errconst", "yze/errlast", "yze/gotostmt"}, ruleIDs(got))
}

func TestFilterWithNoConstraintsKeepsAll(t *testing.T) {
	assert.Len(t, yze.Filter(yze.Registrations(), nil), 14)
}
