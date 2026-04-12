# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0]

### Added

- `wfs.RenameFS` support via S3 CopyObject + DeleteObject.
- `wfs.SyncWriterFile` support (no-op — S3 writes atomically on Close).
- `S3API` interface defined in this package for testability.
- Builder methods: `WithClient`, `WithConfig`, `WithContext`.
- `NewWithClient` convenience constructor.
- Lazy client initialization — `New(bucket)` defers AWS config loading
  until the first operation.
- CI: GitHub Actions with Go version matrix (`1.24`, `stable`),
  golangci-lint, govulncheck.
- Dependabot for weekly Go module and GitHub Actions updates.
- `CHANGELOG.md`.

### Changed

- **Breaking:** Migrated from `aws-sdk-go` (v1) to `aws-sdk-go-v2`.
- **Breaking:** `New(bucket)` no longer takes `context.Context` or
  returns `error`. Use `WithContext` to set a context.
- **Breaking:** `NewWithSession` removed. Use `NewWithClient` or
  `New(bucket).WithConfig(cfg)`.
- **Breaking:** `NewWithAPI` renamed to `NewWithClient`. The parameter
  type changed from `s3iface.S3API` to the `S3API` interface defined in
  this package.
- Upgraded `wfs` dependency from v0.4.0 to v0.5.0.
- Removed `io2` dependency (replaced with internal `lazyReadCloser`).
- Minimum Go version set to 1.24.

## [0.3.0] and earlier

See the git log.

[Unreleased]: https://github.com/mojatter/s3fs/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/mojatter/s3fs/compare/v0.3.0...v0.4.0
