package yze_test

import (
	"testing"

	goyze "github.com/gomatic/go-yze"
	"github.com/gomatic/yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	assert.Equal(t, []string{"yze/go/errconst", "yze/go/gotostmt", "yze/go/namedtypes"}, ruleIDs(regs))
	for _, r := range regs {
		require.NoError(t, r.Validate())
	}
}

func TestFilterByGroupKeepsMatching(t *testing.T) {
	assert.Len(t, yze.Filter(yze.Registrations(), "go", nil), 3)
	assert.Empty(t, yze.Filter(yze.Registrations(), "sql", nil))
}

func TestFilterByCategorySelectsMatching(t *testing.T) {
	got := yze.Filter(yze.Registrations(), "", []goyze.Category{"errors"})
	assert.Equal(t, []string{"yze/go/errconst"}, ruleIDs(got))
}

func TestFilterByMultipleCategories(t *testing.T) {
	got := yze.Filter(yze.Registrations(), "", []goyze.Category{"errors", "patterns"})
	assert.Equal(t, []string{"yze/go/errconst", "yze/go/gotostmt"}, ruleIDs(got))
}

func TestFilterWithNoConstraintsKeepsAll(t *testing.T) {
	assert.Len(t, yze.Filter(yze.Registrations(), "", nil), 3)
}
