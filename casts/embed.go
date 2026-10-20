// Package casts holds the persona library inside the program, so a copy installed with go install can list
// and run the library without the repository.
package casts

import "embed"

// Library is the casts/library folder: one folder per persona.
//
//go:embed library
var Library embed.FS
