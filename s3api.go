package s3fs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/mojatter/io2"
	"github.com/mojatter/wfs"
)

const defaultMaxKeys = int64(1000)

func getMaxKeys(n *int64) int64 {
	i := aws.Int64Value(n)
	if i <= 0 {
		return defaultMaxKeys
	}
	return i
}

// fsS3api provides a simple implementation for mocking on test of s3fs package.
type fsS3api struct {
	s3iface.S3API
	fsys fs.FS
}

var _ s3iface.S3API = (*fsS3api)(nil)

// newFsS3api returns a s3iface.S3API implementation on the provided filesystem.
func newFsS3api(fsys fs.FS) *fsS3api {
	return &fsS3api{
		fsys: fsys,
	}
}

// GetObject API operation for the filesystem.
func (api *fsS3api) GetObject(input *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
	name := path.Join(aws.StringValue(input.Bucket), aws.StringValue(input.Key))
	info, err := fs.Stat(api.fsys, name)
	if err != nil {
		return nil, toS3NoSuckKeyIfNoExist(err)
	}
	if info.IsDir() {
		return nil, toS3NoSuckKeyIfNoExist(fs.ErrNotExist)
	}

	var in io.ReadCloser
	body := &io2.Delegator{}
	body.ReadFunc = func(p []byte) (int, error) {
		if in == nil {
			var err error
			in, err = api.fsys.Open(name)
			if err != nil {
				return 0, err
			}
		}
		return in.Read(p)
	}
	body.CloseFunc = func() error {
		if in != nil {
			return in.Close()
		}
		return nil
	}

	return &s3.GetObjectOutput{
		Body:          body,
		ContentLength: aws.Int64(info.Size()),
		LastModified:  aws.Time(info.ModTime()),
	}, nil
}

// PutObject API operation for the filesystem.
func (api *fsS3api) PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	name := path.Join(aws.StringValue(input.Bucket), aws.StringValue(input.Key))
	output := &s3.PutObjectOutput{}
	f, err := wfs.CreateFile(api.fsys, name, fs.ModePerm)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	io.Copy(f, input.Body)
	return output, nil
}

func (api *fsS3api) namePrefixes(dirPtr, prefixPtr *string) (string, string, error) {
	prefix := aws.StringValue(prefixPtr)
	namePrefix := ""
	dirWithPrefix := path.Join(aws.StringValue(dirPtr), prefix)
	info, err := fs.Stat(api.fsys, dirWithPrefix)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", "", err
		}
		if dirSlash := strings.LastIndex(prefix, "/"); dirSlash != -1 {
			namePrefix = prefix[dirSlash+1:]
			prefix = prefix[:dirSlash]
		} else {
			namePrefix = prefix
			prefix = ""
		}
	} else if !info.IsDir() {
		return "", "", &fs.PathError{Op: "readDir", Path: dirWithPrefix, Err: syscall.ENOTDIR}
	}
	return prefix, namePrefix, nil
}

func (api *fsS3api) readDir(input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	prefix, namePrefix, err := api.namePrefixes(input.Bucket, input.Prefix)
	if err != nil {
		return nil, err
	}
	dir := path.Join(aws.StringValue(input.Bucket), prefix)
	entries, err := fs.ReadDir(api.fsys, dir)
	if err != nil {
		return nil, toS3NoSuckKeyIfNoExist(err)
	}

	output := &s3.ListObjectsV2Output{}
	limit := getMaxKeys(input.MaxKeys)
	after := aws.StringValue(input.StartAfter)
	limited := false
	truncated := false

	for _, entry := range entries {
		name := path.Join(prefix, entry.Name())
		if !strings.HasPrefix(name, namePrefix) {
			continue
		}
		if entry.IsDir() {
			output.CommonPrefixes = append(output.CommonPrefixes, &s3.CommonPrefix{
				Prefix: aws.String(name),
			})
			continue
		}
		if limited {
			truncated = true
			continue
		}
		if after >= name {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, toS3NoSuckKeyIfNoExist(err)
		}
		output.Contents = append(output.Contents, &s3.Object{
			Key:          aws.String(name),
			Size:         aws.Int64(info.Size()),
			LastModified: aws.Time(info.ModTime()),
		})
		limited = (int64(len(output.Contents)) >= limit)
	}

	output.IsTruncated = aws.Bool(truncated)
	return output, nil
}

func (api *fsS3api) walkDir(input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	prefix, namePrefix, err := api.namePrefixes(input.Bucket, input.Prefix)
	if err != nil {
		return nil, err
	}
	root := path.Join(aws.StringValue(input.Bucket), prefix)
	output := &s3.ListObjectsV2Output{}
	limit := getMaxKeys(input.MaxKeys)
	after := aws.StringValue(input.StartAfter)
	limited := false
	truncated := false

	err = fs.WalkDir(api.fsys, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == root || !strings.HasPrefix(name, namePrefix) || d.IsDir() {
			return nil
		}
		name, err = filepath.Rel(aws.StringValue(input.Bucket), name)
		if err != nil {
			return err
		}
		if limited {
			truncated = true
			return fs.SkipDir
		}
		if after >= name {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return toS3NoSuckKeyIfNoExist(err)
		}
		output.Contents = append(output.Contents, &s3.Object{
			Key:          aws.String(name),
			Size:         aws.Int64(info.Size()),
			LastModified: aws.Time(info.ModTime()),
		})
		limited = (int64(len(output.Contents)) >= limit)
		return nil
	})
	if err != nil {
		return nil, err
	}

	output.IsTruncated = aws.Bool(truncated)
	return output, nil
}

// ListObjectsV2 API operation for the filesystem.
func (api *fsS3api) ListObjectsV2(input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	if aws.StringValue(input.Delimiter) == "/" {
		return api.readDir(input)
	}
	return api.walkDir(input)
}

// DeleteObject API operation for the filesystem.
func (api *fsS3api) DeleteObject(input *s3.DeleteObjectInput) (*s3.DeleteObjectOutput, error) {
	name := path.Join(aws.StringValue(input.Bucket), aws.StringValue(input.Key))
	if err := wfs.RemoveFile(api.fsys, name); err != nil {
		return nil, toS3NoSuckKeyIfNoExist(err)
	}
	return &s3.DeleteObjectOutput{}, nil
}

// DeleteObjects API operation for the filesystem.
func (api *fsS3api) DeleteObjects(input *s3.DeleteObjectsInput) (*s3.DeleteObjectsOutput, error) {
	dirs := map[string]interface{}{}
	for _, id := range input.Delete.Objects {
		name := path.Join(aws.StringValue(input.Bucket), aws.StringValue(id.Key))
		if err := wfs.RemoveFile(api.fsys, name); err != nil {
			return nil, toS3NoSuckKeyIfNoExist(err)
		}
		dirs[path.Dir(name)] = nil
	}
	return &s3.DeleteObjectsOutput{}, nil
}
