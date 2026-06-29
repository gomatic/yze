// Package yze is the analyzer catalog for the yze family: it aggregates every
// yze-<name> analyzer's registration and filters the set by category. The
// cmd/yze binary drives this catalog through the go-yze runner.
package yze

import (
	"slices"

	goyze "github.com/gomatic/go-yze"
	anonstruct "github.com/gomatic/yze-anonstruct"
	boolname "github.com/gomatic/yze-boolname"
	ctxfirst "github.com/gomatic/yze-ctxfirst"
	emptyiface "github.com/gomatic/yze-emptyiface"
	errconst "github.com/gomatic/yze-errconst"
	errlast "github.com/gomatic/yze-errlast"
	gotostmt "github.com/gomatic/yze-gotostmt"
	layout "github.com/gomatic/yze-layout"
	namedtypes "github.com/gomatic/yze-namedtypes"
	pkgstd "github.com/gomatic/yze-pkgstd"
	ptrparam "github.com/gomatic/yze-ptrparam"
	ptrrecv "github.com/gomatic/yze-ptrrecv"
	stdlog "github.com/gomatic/yze-stdlog"
	testfile "github.com/gomatic/yze-testfile"
)

// Registrations returns every analyzer in the suite, in stable rule-id order.
func Registrations() []goyze.Registration {
	return []goyze.Registration{
		anonstruct.Registration,
		boolname.Registration,
		ctxfirst.Registration,
		emptyiface.Registration,
		errconst.Registration,
		errlast.Registration,
		gotostmt.Registration,
		layout.Registration,
		namedtypes.Registration,
		pkgstd.Registration,
		ptrparam.Registration,
		ptrrecv.Registration,
		stdlog.Registration,
		testfile.Registration,
	}
}

// Filter selects the registrations matching the given categories. An empty
// category set matches every analyzer; otherwise a registration matches when it
// carries any of the categories.
func Filter(regs []goyze.Registration, categories []goyze.Category) []goyze.Registration {
	out := make([]goyze.Registration, 0, len(regs))
	for _, r := range regs {
		if matchesCategories(r, categories) {
			out = append(out, r)
		}
	}
	return out
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
