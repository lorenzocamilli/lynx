# Pre-upgrade database fixture

`pre-gqlgen-upgrade.db.gz` is a synthetic BoltDB database written by Lynx commit
`974aee587259084c5626ceec045ff0e4e8e8f7ca` (before issue #24's dependency upgrades),
using Go 1.26.10 on linux/amd64. It contains no captured user traffic or secrets.

The fixture includes project settings with a persisted filter AST and scope
regex, a request/response log, and a Sender request with its source-log link and
response. The HTTP GraphQL compatibility test opens it with the upgraded code.
This verifies that existing stored records can be decoded and served, rather
than just round-tripping records written by the new version.

To regenerate from the repository root, using a new temporary directory:

```sh
repo=$(pwd)
baseline=$(mktemp -d)
git archive 974aee587259084c5626ceec045ff0e4e8e8f7ca go.mod go.sum pkg | tar -x -C "$baseline"
cp pkg/api/testdata/generate_legacy.go "$baseline/generate_legacy.go"
(cd "$baseline" && go run ./generate_legacy.go "$baseline/legacy.db")
gzip -n -c "$baseline/legacy.db" > "$repo/pkg/api/testdata/pre-gqlgen-upgrade.db.gz"
```

Keep the baseline revision fixed when regenerating this fixture. BoltDB page
layout is an implementation detail; compatibility assertions inspect decoded
records, not the compressed file's checksum.
