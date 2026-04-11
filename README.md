# Mataki Platform

Shared Go library for all Mataki products.

## Install

    go get github.com/mataki-dev/platform

Requires Go 1.22+.

## Packages

| Package                                          | Purpose                                                                                        |
| ------------------------------------------------ | ---------------------------------------------------------------------------------------------- |
| [errors](./errors/README.md)                     | Semantic error types with infrastructure mappers and Huma integration                          |
| [search](./search/README.md)                     | Search endpoint infrastructure: validation, SQL generation, cursor pagination, full-text search |
| [strongbox](./strongbox/README.md)               | Multi-client, multi-tenant secret storage with envelope encryption                             |

## Development

    # Run tests
    go test ./...

    # Run tests with race detector
    go test -race ./...

    # Run tests with coverage
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out

    # Vet
    go vet ./...

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). This project uses [Conventional Commits](https://www.conventionalcommits.org/) and releases are cut automatically from `main`.

## License

MIT — see [LICENSE](./LICENSE).
