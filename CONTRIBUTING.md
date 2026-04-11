# Contributing to Mataki Platform

Thanks for your interest in contributing.

## Commit messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/).
Releases are cut automatically from `main` by semantic-release based on commit
messages:

| Type                            | Version bump | Example                                         |
| ------------------------------- | ------------ | ----------------------------------------------- |
| `feat`                          | minor        | `feat(search): add ilike_any operator`          |
| `fix`                           | patch        | `fix(errors): preserve wrapped error in Unwrap` |
| `feat!` / `BREAKING CHANGE:`    | major        | `feat(strongbox)!: rename Store.Put to Create`  |
| `docs`, `chore`, `refactor`, `test`, `perf` | none | `chore: bump go version`                    |

## Pull request flow

1. Branch from `main`
2. Make your changes; add tests
3. Run `go test -race ./...` locally
4. Open a PR against `main`
5. CI must pass before merge
6. Squash-merge (single conventional commit message)

## Tests

Required before merge:

```
go vet ./...
go test -race ./...
```

## File headers

All new `.go` files must include the standard header:

```
// Copyright 2026 Mataki Labs LLC
// SPDX-License-Identifier: MIT
```

Copy from an existing file.
