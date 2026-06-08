# Changelog

All notable changes to this project are documented in this file.
This project adheres to [Semantic Versioning](https://semver.org/).

## [1.1.0]

- Move the module to the `wijnberg-net` organization: the import path is now `github.com/wijnberg-net/vsdx-go`. Update your imports accordingly. Existing `github.com/michelwijnberg/vsdx-go` releases keep resolving via GitHub's redirect.

## [1.0.4]

- Re-audit and correct the feature-support matrix: fill patterns 2-24 render as SVG patterns (previously mis-documented as solid).
- Remove references to internal documents not shipped with the library.

## [1.0.3]

- Stop tracking generated test output (`tests/out/`); it is now git-ignored.
- Minor lint cleanup (`t++` / `t--` increment style).

## [1.0.2]

- Apply `gofmt -s` to the entire codebase (formatting only; no behavior change).

## [1.0.1]

- Add runnable examples (`examples/`) and package-level example documentation.
- Add GitHub Actions CI (build, vet, test).
- Add README badges and this changelog.

## [1.0.0]

Initial public release.

- Read, edit, and write Microsoft Visio `.vsdx` files.
- High-fidelity SVG rendering: geometry, connectors, arrows, line styles,
  fills and gradients, drop shadows, text, transforms, and nested groups.
- Master shapes, stencils (`.vssx`), themes, variants, and stylesheets.
- Formula evaluation, A\* connector auto-routing, templating, schema
  validation, and file diffing.
