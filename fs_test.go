package s3fs

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/aws/aws-sdk-go/service/s3"
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

func (m *mockFSS3API) GetObject(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fsS3api.GetObject(input)
}

func (m *mockFSS3API) PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fsS3api.PutObject(input)
}

func (m *mockFSS3API) ListObjectsV2(input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.fsS3api.ListObjectsV2(input)
}

func TestFS(t *testing.T) {
	fsys := NewWithAPI("testdata", newMockFSS3APITesting(t))
	if err := fstest.TestFS(fsys, "dir0", "dir0/file01.txt"); err != nil {
		t.Errorf("Error testing/fstest: %+v", err)
	}
}

func TestWriteFileFS(t *testing.T) {
	fsys := NewWithAPI("testdata", newMockFSS3APITesting(t))
	tmpDir := "test"
	if err := wfs.MkdirAll(fsys, tmpDir, fs.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := wfstest.TestWriteFileFS(fsys, tmpDir); err != nil {
		t.Errorf("Error wfstest: %+v", err)
	}
}
