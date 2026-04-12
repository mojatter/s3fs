package s3fs

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mojatter/wfs"
	"github.com/mojatter/wfs/memfs"
	"github.com/mojatter/wfs/osfs"
	"github.com/mojatter/wfs/wfstest"
)

func newMemFSTest() (*memfs.MemFS, error) {
	osFsys := osfs.New(".")
	memFsys := memfs.New()
	err := wfs.CopyFS(memFsys, osFsys, "testdata")
	if err != nil {
		return nil, err
	}
	return memFsys, nil
}

func newMemFSTesting(t *testing.T) *memfs.MemFS {
	fsys, err := newMemFSTest()
	if err != nil {
		t.Fatal(err)
	}
	return fsys
}

type mockFSS3API struct {
	*fsS3api
	err error
}

func newMockFSS3API() (*mockFSS3API, error) {
	fsys, err := newMemFSTest()
	if err != nil {
		return nil, err
	}
	return &mockFSS3API{
		fsS3api: newFsS3api(fsys),
	}, nil
}

func newMockFSS3APITesting(t *testing.T) *mockFSS3API {
	api, err := newMockFSS3API()
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func (m *mockFSS3API) GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fsS3api.GetObject(ctx, input, optFns...)
}

func (m *mockFSS3API) PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fsS3api.PutObject(ctx, input, optFns...)
}

func (m *mockFSS3API) CopyObject(ctx context.Context, input *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fsS3api.CopyObject(ctx, input, optFns...)
}

func (m *mockFSS3API) ListObjectsV2(ctx context.Context, input *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fsS3api.ListObjectsV2(ctx, input, optFns...)
}

func TestFS(t *testing.T) {
	fsys := NewWithClient("testdata", newMockFSS3APITesting(t))
	if err := fstest.TestFS(fsys, "dir0", "dir0/file01.txt"); err != nil {
		t.Errorf("Error testing/fstest: %+v", err)
	}
}

func TestWriteFileFS(t *testing.T) {
	fsys := NewWithClient("testdata", newMockFSS3APITesting(t))
	tmpDir := "test"
	if err := wfs.MkdirAll(fsys, tmpDir, fs.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := wfstest.TestWriteFileFS(fsys, tmpDir); err != nil {
		t.Errorf("Error wfstest: %+v", err)
	}
}

func TestRenameFS(t *testing.T) {
	fsys := NewWithClient("testdata", newMockFSS3APITesting(t))
	tmpDir := "test_rename"
	if err := wfs.MkdirAll(fsys, tmpDir, fs.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := wfstest.TestRenameFS(fsys, tmpDir); err != nil {
		t.Errorf("Error wfstest: %+v", err)
	}
}
