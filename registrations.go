// Package yze is the analyzer catalog for the yze family: it aggregates every
// yze-<name> analyzer's registration and filters the set by category. The
// cmd/yze binary drives this catalog through the go-yze runner.
package yze

import (
	"slices"

	goyze "github.com/gomatic/go-yze"
	anonstruct "github.com/gomatic/yze-go-anonstruct"
	boolname "github.com/gomatic/yze-go-boolname"
	cliv3 "github.com/gomatic/yze-go-cliv3"
	cliversion "github.com/gomatic/yze-go-cliversion"
	ctxfirst "github.com/gomatic/yze-go-ctxfirst"
	emptyiface "github.com/gomatic/yze-go-emptyiface"
	errconst "github.com/gomatic/yze-go-errconst"
	errlast "github.com/gomatic/yze-go-errlast"
	gotostmt "github.com/gomatic/yze-go-gotostmt"
	layout "github.com/gomatic/yze-go-layout"
	namedtypes "github.com/gomatic/yze-go-namedtypes"
	pkgstd "github.com/gomatic/yze-go-pkgstd"
	ptrparam "github.com/gomatic/yze-go-ptrparam"
	ptrrecv "github.com/gomatic/yze-go-ptrrecv"
	stdlog "github.com/gomatic/yze-go-stdlog"
	testfile "github.com/gomatic/yze-go-testfile"
)

// Registrations returns every analyzer in the suite, in stable rule-id order.
func Registrations() []goyze.Registration {
	return []goyze.Registration{
		anonstruct.Registration,
		boolname.Registration,
		cliv3.Registration,
		cliversion.Registration,
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
