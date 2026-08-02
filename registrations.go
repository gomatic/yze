// Package yze is the analyzer catalog for the yze family: it aggregates every
// yze-<name> analyzer's registration and filters the set by category. The
// cmd/yze binary drives this catalog through the go-yze runner.
package yze

import (
	"slices"

	goyze "github.com/gomatic/go-yze"
	anonstruct "github.com/gomatic/yze-go-anonstruct"
	boolname "github.com/gomatic/yze-go-boolname"
	cliapp "github.com/gomatic/yze-go-cliapp"
	clidomain "github.com/gomatic/yze-go-clidomain"
	cliflags "github.com/gomatic/yze-go-cliflags"
	cliv3 "github.com/gomatic/yze-go-cliv3"
	cliversion "github.com/gomatic/yze-go-cliversion"
	ctxfirst "github.com/gomatic/yze-go-ctxfirst"
	emptyiface "github.com/gomatic/yze-go-emptyiface"
	errconst "github.com/gomatic/yze-go-errconst"
	errlast "github.com/gomatic/yze-go-errlast"
	errtest "github.com/gomatic/yze-go-errtest"
	errtested "github.com/gomatic/yze-go-errtested"
	filesize "github.com/gomatic/yze-go-filesize"
	globalvar "github.com/gomatic/yze-go-globalvar"
	gotostmt "github.com/gomatic/yze-go-gotostmt"
	invariant "github.com/gomatic/yze-go-invariant"
	jsontag "github.com/gomatic/yze-go-jsontag"
	namedtypes "github.com/gomatic/yze-go-namedtypes"
	noinit "github.com/gomatic/yze-go-noinit"
	nopanic "github.com/gomatic/yze-go-nopanic"
	ptrparam "github.com/gomatic/yze-go-ptrparam"
	ptrrecv "github.com/gomatic/yze-go-ptrrecv"
	slogkv "github.com/gomatic/yze-go-slogkv"
	stdlog "github.com/gomatic/yze-go-stdlog"
	testfile "github.com/gomatic/yze-go-testfile"
	valuector "github.com/gomatic/yze-go-valuector"
)

// sourceOnly names the analyzers whose rules are about PRODUCTION design and are
// therefore wrong when applied to test code.
//
// Test code is a different kind of code. A table-driven test's anonymous struct
// is the Go idiom, not a defect; `want` is a fine boolean field name; a test
// double may construct an ad-hoc error; a test helper may take a bare string.
// Enforcing the production shape rules there would ban the table-driven test —
// the very idiom the testing standard is built around.
//
// This mirrors the long-standing golangci policy of giving test files a lighter
// touch. yze never needed the equivalent because its driver could not see test
// files at all; once it could, 74% of the newly-reported findings across the
// fleet came from these rules meeting test code for the first time — a policy
// change nobody had decided.
//
// The architectural rules (cliapp, cliflags, cliversion) belong here for the
// same reason as the shape rules: they judge how the PRODUCTION program is
// wired and laid out. A test may build a bare urfave/cli command as a
// flag-parsing harness — it has no business setting a Version or binding env
// sources — and a command package's test may import its domain package however
// it likes. Judging those reported 37 findings across the fleet, every one of
// them in a _test.go file. (Tree correspondence itself moved out of the suite
// entirely: stickler/clilayout checks it at the whole-repo layer.)
//
// Analyzers absent from this set report everywhere, which is the default: a
// scope is opted into, never inherited by accident.
var sourceOnly = map[goyze.AnalyzerName]bool{
	"anonstruct": true,
	"boolname":   true,
	"cliapp":     true,
	"clidomain":  true,
	"cliflags":   true,
	"cliversion": true,
	"emptyiface": true,
	"errconst":   true,
	"namedtypes": true,
	"ptrparam":   true,
	"ptrrecv":    true,
	"valuector":  true,
}

// Registrations returns every analyzer in the suite, in stable rule-id order,
// each carrying the test-file scope its rule warrants.
func Registrations() []goyze.Registration {
	return withScopes([]goyze.Registration{
		anonstruct.Registration,
		boolname.Registration,
		cliapp.Registration,
		clidomain.Registration,
		cliflags.Registration,
		cliv3.Registration,
		cliversion.Registration,
		ctxfirst.Registration,
		emptyiface.Registration,
		errconst.Registration,
		errlast.Registration,
		errtest.Registration,
		errtested.Registration,
		filesize.Registration,
		globalvar.Registration,
		gotostmt.Registration,
		invariant.Registration,
		jsontag.Registration,
		namedtypes.Registration,
		noinit.Registration,
		nopanic.Registration,
		ptrparam.Registration,
		ptrrecv.Registration,
		slogkv.Registration,
		stdlog.Registration,
		testfile.Registration,
		valuector.Registration,
	})
}

// withScopes stamps the test-file policy onto each registration.
func withScopes(regs []goyze.Registration) []goyze.Registration {
	out := make([]goyze.Registration, 0, len(regs))
	for _, r := range regs {
		if sourceOnly[r.Name] {
			r = r.WithTestScope(goyze.TestScopeSourceOnly)
		}
		out = append(out, r)
	}
	return out
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
