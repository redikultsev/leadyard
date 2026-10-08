// Package spec embeds the machine-readable parts of the leadyard spec.
package spec

import _ "embed"

// Transitions is spec/transitions.yaml.
//
//go:embed transitions.yaml
var Transitions []byte
