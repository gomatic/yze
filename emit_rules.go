package yze

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
)

// ErrUnknownRuleFormat reports a rule-export format the aggregator does not support.
const ErrUnknownRuleFormat errs.Const = "unknown rule-export format"

// RuleFormat names a way the suite's rule catalog is exported. The catalog is built
// from catalog metadata (id, name, doc, url, categories) — never from analyzer
// logic — so it carries no per-analyzer machinery.
type RuleFormat string

// The rule-export formats `yze --emit-rules` supports.
const (
	// RuleFormatSARIF exports a SARIF 2.1.0 tool.driver.rules catalog for code
	// scanning / editor rule registration.
	RuleFormatSARIF RuleFormat = "sarif"
	// RuleFormatGrit exports a GritQL-style markdown registry. The analyzers are
	// AST-based, not pattern rewrites, so this is a documentation catalog (rule id,
	// description, docs link, categories) — not executable rewrite patterns.
	RuleFormatGrit RuleFormat = "grit"
)

// sarifSchemaURL is the SARIF 2.1.0 JSON-schema URL stamped into the log.
const sarifSchemaURL = "https://json.schemastore.org/sarif-2.1.0.json"

// Rule is one catalog entry: the language-neutral metadata a rule export carries.
// Both the Go analyzer registrations (via [GoRules]) and the SQL analyzers (via
// [SQLRules]) reduce to this shape, so the exported catalog always describes the
// whole suite — every rule id a diagnostic can carry has a catalog entry.
type Rule struct {
	ID         string
	Name       string
	Doc        string
	URL        string
	Categories []goyze.Category
}

// GoRules maps the Go analyzer registrations onto their catalog rules.
func GoRules(regs []goyze.Registration) []Rule {
	rules := make([]Rule, 0, len(regs))
	for _, reg := range regs {
		rules = append(rules, Rule{
			ID:         reg.RuleID(),
			Name:       string(reg.Name),
			Doc:        docOf(reg),
			URL:        string(reg.URL),
			Categories: reg.Categories,
		})
	}
	return rules
}

// SQLRules maps the SQL analyzers onto their catalog rules, under the same
// "yze/<name>" rule-id scheme their diagnostics carry.
func SQLRules(analyzers []SQLAnalyzer) []Rule {
	rules := make([]Rule, 0, len(analyzers))
	for _, a := range analyzers {
		rules = append(rules, Rule{
			ID:         a.RuleID(),
			Name:       string(a.Name),
			Doc:        a.Doc,
			URL:        string(a.URL),
			Categories: a.Categories,
		})
	}
	return rules
}

// CatalogRules merges the Go and SQL analyzers' rules into the one catalog
// `--emit-rules` exports, sorted by rule id so the export order is deterministic
// regardless of which language contributed an entry.
func CatalogRules(regs []goyze.Registration, analyzers []SQLAnalyzer) []Rule {
	rules := append(GoRules(regs), SQLRules(analyzers)...)
	slices.SortFunc(rules, func(a, b Rule) int { return strings.Compare(a.ID, b.ID) })
	return rules
}

// EmitRules writes the rule catalog to w in the named format.
func EmitRules(w io.Writer, format RuleFormat, rules []Rule) error {
	switch format {
	case RuleFormatSARIF:
		return emitSARIFRules(w, rules)
	case RuleFormatGrit:
		return emitGritRules(w, rules)
	default:
		return ErrUnknownRuleFormat.With(nil, "format", string(format))
	}
}

// sarifLog is the minimal SARIF 2.1.0 envelope carrying a rules-only driver.
type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool sarifTool `json:"tool"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name  string      `json:"name"`
	Rules []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	ShortDescription sarifText      `json:"shortDescription"`
	HelpURI          string         `json:"helpUri,omitempty"`
	Properties       sarifRuleProps `json:"properties"`
}

type sarifText struct {
	Text string `json:"text"`
}

type sarifRuleProps struct {
	Tags []string `json:"tags"`
}

// emitSARIFRules writes the catalog as a SARIF log whose single run carries every
// rule under tool.driver.rules.
func emitSARIFRules(w io.Writer, rules []Rule) error {
	out := make([]sarifRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, sarifRuleOf(rule))
	}
	log := sarifLog{
		Schema:  sarifSchemaURL,
		Version: "2.1.0",
		Runs:    []sarifRun{{Tool: sarifTool{Driver: sarifDriver{Name: "yze", Rules: out}}}},
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(log)
}

// sarifRuleOf maps one catalog rule's metadata onto a SARIF rule.
func sarifRuleOf(rule Rule) sarifRule {
	return sarifRule{
		ID:               rule.ID,
		Name:             rule.Name,
		ShortDescription: sarifText{Text: rule.Doc},
		HelpURI:          rule.URL,
		Properties:       sarifRuleProps{Tags: tagsOf(rule.Categories)},
	}
}

// emitGritRules writes the catalog as a GritQL-style markdown registry: one entry
// per rule with its id, description, docs link, and categories.
func emitGritRules(w io.Writer, rules []Rule) error {
	if _, err := fmt.Fprint(w, "# yze rule catalog\n\n"); err != nil {
		return err
	}
	for _, rule := range rules {
		if err := writeGritRule(w, rule); err != nil {
			return err
		}
	}
	return nil
}

// writeGritRule writes one rule's markdown registry entry.
func writeGritRule(w io.Writer, rule Rule) error {
	_, err := fmt.Fprintf(w, "## `%s`\n\n%s\n\n- docs: %s\n- categories: %s\n\n",
		rule.ID, rule.Doc, rule.URL, categoryList(rule.Categories))
	return err
}

// docOf returns the analyzer's doc string, or an empty string when absent.
func docOf(reg goyze.Registration) string {
	if reg.Analyzer == nil {
		return ""
	}
	return reg.Analyzer.Doc
}

// tagsOf converts categories to plain string tags for SARIF properties.
func tagsOf(categories []goyze.Category) []string {
	tags := make([]string, 0, len(categories))
	for _, c := range categories {
		tags = append(tags, string(c))
	}
	return tags
}

// categoryList renders categories as a comma-separated string, or "none".
func categoryList(categories []goyze.Category) string {
	if len(categories) == 0 {
		return "none"
	}
	out := ""
	for i, c := range categories {
		if i > 0 {
			out += ", "
		}
		out += string(c)
	}
	return out
}
