package report

import _ "embed"

//go:generate cp ../../schema/report.schema.json report.schema.json

//go:embed report.schema.json
var schemaJSON []byte

// SchemaJSON returns the embedded report schema, for a check against the repository copy.
func SchemaJSON() []byte { return schemaJSON }
