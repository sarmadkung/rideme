# tests/

Cross-cutting tests that need real infrastructure. Package-level unit tests live
beside the code they cover.

Integration tests are behind the `integration` build tag so `go test ./...` stays
fast and dependency-free:

    make infra-up          # from the repository root
    go test -tags=integration ./tests/...
