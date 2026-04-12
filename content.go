package s3fs

import (
	"io/fs"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type content struct {
	name    string
	isDir   bool
	size    int64
	modTime time.Time
}

var (
	_ fs.DirEntry = (*content)(nil)
	_ fs.FileInfo = (*content)(nil)
)

func newDirContent(prefix string) *content {
	return &content{
		name:  path.Base(prefix),
		isDir: true,
	}
}

func newFileContent(o s3types.Object) *content {
	return &content{
		name:    path.Base(aws.ToString(o.Key)),
		size:    aws.ToInt64(o.Size),
		modTime: aws.ToTime(o.LastModified),
	}
}

func (c *content) Name() string {
	return c.name
}

func (c *content) Size() int64 {
	return c.size
}

// Mode returns if this content is directory then fs.ModePerm | fs.ModeDir otherwise fs.ModePerm.
func (c *content) Mode() fs.FileMode {
	if c.isDir {
		return fs.ModePerm | fs.ModeDir
	}
	return fs.ModePerm
}

func (c *content) ModTime() time.Time {
	return c.modTime
}

func (c *content) IsDir() bool {
	return c.isDir
}

func (c *content) Sys() interface{} {
	return nil
}

func (c *content) Type() fs.FileMode {
	return c.Mode() & fs.ModeType
}

func (c *content) Info() (fs.FileInfo, error) {
	return c, nil
}
