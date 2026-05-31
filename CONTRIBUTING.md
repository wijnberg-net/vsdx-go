# Contributing

Contributions are welcome!

## Development Setup

```bash
git clone https://github.com/MichelW6667/vsdx-go.git
cd vsdx-go
go mod download
```

## Running Tests

```bash
go test ./vsdx/... -v
```

Test fixtures are `.vsdx` files in the `tests/` directory. When adding new features, include test cases using existing fixtures where possible.

## Project Structure

- `vsdx/` - Main Go package with all library code (18 files)
- `tests/` - Test fixture `.vsdx` files
- `docs/` - Reference documentation (MS-VSDX format spec)

See `README.md` for a detailed file-by-file breakdown.

## Guidelines

- Follow Go conventions and idioms
- Use `github.com/beevik/etree` for XML parsing
- Use cell name constants from `cellname.go` (e.g., `CellPinX` instead of `"PinX"`)
- Use sentinel errors from `errors.go` where appropriate
- Return `error` values instead of silently ignoring failures
- Add tests for new features
- Run `go vet ./...` and `go test ./...` before submitting
