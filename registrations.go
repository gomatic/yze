// Package yze is the analyzer catalog for the yze family: it aggregates every
// yze-<group>-<name> analyzer's registration and filters the set by group and
// category. The cmd/yze binary drives this catalog through the go-yze runner.
package yze

import (
	"slices"

	goyze "github.com/gomatic/go-yze"
	anonstruct "github.com/gomatic/yze-go-anonstruct"
	boolname "github.com/gomatic/yze-go-boolname"
	emptyiface "github.com/gomatic/yze-go-emptyiface"
	errconst "github.com/gomatic/yze-go-errconst"
	gotostmt "github.com/gomatic/yze-go-gotostmt"
	layout "github.com/gomatic/yze-go-layout"
	namedtypes "github.com/gomatic/yze-go-namedtypes"
	pkgstd "github.com/gomatic/yze-go-pkgstd"
	ptrparam "github.com/gomatic/yze-go-ptrparam"
	ptrrecv "github.com/gomatic/yze-go-ptrrecv"
)

// Registrations returns every analyzer in the suite, in stable rule-id order.
func Registrations() []goyze.Registration {
	return []goyze.Registration{
		anonstruct.Registration,
		boolname.Registration,
		emptyiface.Registration,
		errconst.Registration,
		gotostmt.Registration,
		layout.Registration,
		namedtypes.Registration,
		pkgstd.Registration,
		ptrparam.Registration,
		ptrrecv.Registration,
	}
}

// Filter selects the registrations matching the given group and categories. An
// empty group matches every group; an empty category set matches every category;
// a registration matches the category set when it carries any of the categories.
func Filter(regs []goyze.Registration, group goyze.Group, categories []goyze.Category) []goyze.Registration {
	out := make([]goyze.Registration, 0, len(regs))
	for _, r := range regs {
		if matchesGroup(r, group) && matchesCategories(r, categories) {
			out = append(out, r)
		}
	}
	return out
}

func matchesGroup(r goyze.Registration, group goyze.Group) bool {
	return group == "" || r.Group == group
}

func matchesCategories(r goyze.Registration, categories []goyze.Category) bool {
	if len(categories) == 0 {
		return true
	}
	for _, want := range categories {
		if slices.Contains(r.Categories, want) {
			return true
		}
	}
	return false
}
