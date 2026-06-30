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
	globalvar "github.com/gomatic/yze-go-globalvar"
	gotostmt "github.com/gomatic/yze-go-gotostmt"
	jsontag "github.com/gomatic/yze-go-jsontag"
	layout "github.com/gomatic/yze-go-layout"
	namedtypes "github.com/gomatic/yze-go-namedtypes"
	noinit "github.com/gomatic/yze-go-noinit"
	nopanic "github.com/gomatic/yze-go-nopanic"
	pkgstd "github.com/gomatic/yze-go-pkgstd"
	ptrparam "github.com/gomatic/yze-go-ptrparam"
	ptrrecv "github.com/gomatic/yze-go-ptrrecv"
	slogkv "github.com/gomatic/yze-go-slogkv"
	stdlog "github.com/gomatic/yze-go-stdlog"
	testfile "github.com/gomatic/yze-go-testfile"
	valuector "github.com/gomatic/yze-go-valuector"
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
		globalvar.Registration,
		gotostmt.Registration,
		jsontag.Registration,
		layout.Registration,
		namedtypes.Registration,
		noinit.Registration,
		nopanic.Registration,
		pkgstd.Registration,
		ptrparam.Registration,
		ptrrecv.Registration,
		slogkv.Registration,
		stdlog.Registration,
		testfile.Registration,
		valuector.Registration,
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
