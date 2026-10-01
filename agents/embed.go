// Package agents carries the setup skill inside the binary, so a release
// download can install it without the repository.
package agents

import "embed"

//go:embed setup.md claude/SKILL.md opencode/SKILL.md pi/SKILL.md
var Files embed.FS
