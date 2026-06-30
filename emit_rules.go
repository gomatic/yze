package yze

import (
	"encoding/json"
	"fmt"
	"io"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
)

// ErrUnknownRuleFormat reports a rule-export format the aggregator does not support.
const ErrUnknownRuleFormat errs.Const = "unknown rule-export format"

// RuleFormat names a way the suite's rule catalog is exported. The catalog is built
// from registration metadata (id, name, doc, url, categories) — never from analyzer
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

// EmitRules writes the suite's rule catalog to w in the named format.
func EmitRules(w io.Writer, format RuleFormat, regs []goyze.Registration) error {
	switch format {
	case RuleFormatSARIF:
		return emitSARIFRules(w, regs)
	case RuleFormatGrit:
		return emitGritRules(w, regs)
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
func emitSARIFRules(w io.Writer, regs []goyze.Registration) error {
	rules := make([]sarifRule, 0, len(regs))
	for _, reg := range regs {
		rules = append(rules, sarifRuleOf(reg))
	}
	log := sarifLog{
		Schema:  sarifSchemaURL,
		Version: "2.1.0",
		Runs:    []sarifRun{{Tool: sarifTool{Driver: sarifDriver{Name: "yze", Rules: rules}}}},
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(log)
}

// sarifRuleOf maps one registration's metadata onto a SARIF rule.
func sarifRuleOf(reg goyze.Registration) sarifRule {
	return sarifRule{
		ID:               reg.RuleID(),
		Name:             string(reg.Name),
		ShortDescription: sarifText{Text: docOf(reg)},
		HelpURI:          string(reg.URL),
		Properties:       sarifRuleProps{Tags: tagsOf(reg.Categories)},
	}
}

// emitGritRules writes the catalog as a GritQL-style markdown registry: one entry
// per rule with its id, description, docs link, and categories.
func emitGritRules(w io.Writer, regs []goyze.Registration) error {
	if _, err := fmt.Fprint(w, "# yze rule catalog\n\n"); err != nil {
		return err
	}
	for _, reg := range regs {
		if err := writeGritRule(w, reg); err != nil {
			return err
		}
	}
	return nil
}

// writeGritRule writes one rule's markdown registry entry.
func writeGritRule(w io.Writer, reg goyze.Registration) error {
	_, err := fmt.Fprintf(w, "## `%s`\n\n%s\n\n- docs: %s\n- categories: %s\n\n",
		reg.RuleID(), docOf(reg), string(reg.URL), categoryList(reg.Categories))
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
