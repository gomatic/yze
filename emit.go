package yze

import (
	"encoding/json"
	"fmt"
	"io"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
)

// ErrUnknownFormat reports an output format the aggregator does not support.
const ErrUnknownFormat errs.Const = "unknown output format"

// Format names a way the aggregator serializes a report.
type Format string

// The output formats cmd/yze supports. Richer formats (sarif, github) live in
// the stickler runner, which consumes the stickler-json emitted here.
const (
	FormatSticklerJSON Format = "stickler-json"
	FormatText         Format = "text"
)

// Emit writes the report to w in the named format.
func Emit(w io.Writer, format Format, report goyze.Report) error {
	switch format {
	case FormatSticklerJSON:
		return json.NewEncoder(w).Encode(report)
	case FormatText:
		return emitText(w, report)
	default:
		return ErrUnknownFormat.With(nil, "format", string(format))
	}
}

// emitText writes one human-readable line per diagnostic.
func emitText(w io.Writer, report goyze.Report) error {
	for _, d := range report.Diagnostics {
		if _, err := fmt.Fprintf(w, "%s:%d:%d: %s (%s)\n", d.Path, d.Line, d.Col, d.Message, d.Rule); err != nil {
			return err
		}
	}
	return nil
}
