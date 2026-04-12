# s3fs

[![PkgGoDev](https://pkg.go.dev/badge/github.com/mojatter/s3fs)](https://pkg.go.dev/github.com/mojatter/s3fs)
[![Report Card](https://goreportcard.com/badge/github.com/mojatter/s3fs)](https://goreportcard.com/report/github.com/mojatter/s3fs)
[![Tests](https://github.com/mojatter/s3fs/actions/workflows/tests.yaml/badge.svg)](https://github.com/mojatter/s3fs/actions/workflows/tests.yaml)

Package s3fs provides an implementation of [wfs](https://github.com/mojatter/wfs) for S3.

Requires Go 1.24 or later.

## Examples

### ReadDir

```go
package main

import (
  "fmt"
  "io/fs"
  "log"

  "github.com/mojatter/s3fs"
)

func main() {
  fsys := s3fs.New("<your-bucket>")
  entries, err := fs.ReadDir(fsys, ".")
  if err != nil {
    log.Fatal(err)
  }
  for _, entry := range entries {
    fmt.Println(entry.Name())
  }
}
```

### WriteFile

```go
package main

import (
  "io/fs"
  "log"

  "github.com/mojatter/s3fs"
  "github.com/mojatter/wfs"
)

func main() {
  fsys := s3fs.New("<your-bucket>")
  _, err := wfs.WriteFile(fsys, "test.txt", []byte(`Hello`), fs.ModePerm)
  if err != nil {
    log.Fatal(err)
  }
}
```

### Explicit client

When you need to control the AWS configuration (region, credentials,
custom endpoint, etc.), construct the S3 client yourself and pass it in:

```go
package main

import (
  "context"
  "log"

  "github.com/aws/aws-sdk-go-v2/config"
  "github.com/aws/aws-sdk-go-v2/service/s3"
  "github.com/mojatter/s3fs"
)

func main() {
  ctx := context.Background()
  cfg, err := config.LoadDefaultConfig(ctx)
  if err != nil {
    log.Fatal(err)
  }
  fsys := s3fs.NewWithClient("<your-bucket>", s3.NewFromConfig(cfg)).
    WithContext(ctx)
  // use fsys ...
  _ = fsys
}
```

## Capability layers

s3fs implements the following [wfs](https://github.com/mojatter/wfs)
capability interfaces:

| Capability | Interface | Notes |
| --- | --- | --- |
| Read | `fs.FS`, `fs.GlobFS`, `fs.ReadDirFS`, `fs.ReadFileFS`, `fs.StatFS`, `fs.SubFS` | |
| Write | `wfs.WriteFileFS` | `MkdirAll` is a no-op (S3 has no directories) |
| Remove | `wfs.RemoveFileFS` | |
| Rename | `wfs.RenameFS` | Implemented via CopyObject + DeleteObject |
| Sync | `wfs.SyncWriterFile` | No-op (S3 writes atomically on Close) |

## Tests

s3fs can pass TestFS in `testing/fstest`.

```go
import (
  "testing/fstest"
  "github.com/mojatter/s3fs"
)

// ...

fsys := s3fs.New("<your-bucket>")
if err := fstest.TestFS(fsys, "<your-expected>"); err != nil {
  t.Errorf("Error testing/fstest: %+v", err)
}
```

## Integration tests

```sh
FSTEST_BUCKET="<your-bucket>" \
FSTEST_EXPECTED="<your-expected>" \
  go test -tags integtest ./...
```
