// Package quickstart embeds the quickstart's compose bundle so conduitctl
// can run it without a checkout: compose.yml and the files it mounts.
package quickstart

import "embed"

// Bundle holds compose.yml, tempo.yaml and clickhouse/*.xml.
//
//go:embed compose.yml tempo.yaml clickhouse/*.xml
var Bundle embed.FS
