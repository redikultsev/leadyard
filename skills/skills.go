// Package skills embeds leadyard's reference skills.
package skills

import "embed"

// FS holds <name>/SKILL.md for every reference skill.
//
//go:embed */SKILL.md
var FS embed.FS
