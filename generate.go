// Package iplist provides a compact IPv4 IP-to-label database and lookup APIs.
package iplist

// Regenerate the prebuilt database file from the repository data directory.
//
// This follows the same pattern as golang.org/x/net/publicsuffix: keep the
// human-editable sources (data/, docs/) in the repo and use `go generate`
// to produce the derived binary artifact.
//
//go:generate go run ./internal/cmd/gen-names -country ./docs/country.md -cncity ./docs/cncity.md -out ./docs_names_gen.go
//go:generate go run ./cmd/iplist build -data ./data -out ./iplist.db
