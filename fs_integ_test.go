//go:build integtest
// +build integtest

package s3fs

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFSIntegration(t *testing.T) {
	bucket := os.Getenv("FSTEST_BUCKET")
	expected := os.Getenv("FSTEST_EXPECTED")
	if bucket == "" || expected == "" {
		t.Fatalf("Require ENV FSTEST_BUCKET=%s FSTEST_EXPECTED=%s", bucket, expected)
	}

	fsys, err := New(context.Background(), bucket)
	if err != nil {
		t.Fatalf("Failed to create S3FS: %v", err)
	}
	if err := fstest.TestFS(fsys, strings.Split(expected, ",")...); err != nil {
		t.Errorf("Error testing/fstest: %+v", err)
	}
}
