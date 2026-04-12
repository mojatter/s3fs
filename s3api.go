package s3fs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/mojatter/wfs"
)

// lazyReadCloser is a simple io.ReadCloser backed by function delegates.
type lazyReadCloser struct {
	readFunc  func(p []byte) (int, error)
	closeFunc func() error
}

func (rc *lazyReadCloser) Read(p []byte) (int, error)  { return rc.readFunc(p) }
func (rc *lazyReadCloser) Close() error                { return rc.closeFunc() }

const defaultMaxKeys = int32(1000)

func getMaxKeys(n *int32) int32 {
	i := aws.ToInt32(n)
	if i <= 0 {
		return defaultMaxKeys
	}
	return i
}

// fsS3api provides a simple implementation for mocking on test of s3fs package.
type fsS3api struct {
	fsys fs.FS
}

var _ S3API = (*fsS3api)(nil)

// newFsS3api returns a S3API implementation on the provided filesystem.
func newFsS3api(fsys fs.FS) *fsS3api {
	return &fsS3api{
		fsys: fsys,
	}
}

// GetObject API operation for the filesystem.
func (api *fsS3api) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	name := path.Join(aws.ToString(input.Bucket), aws.ToString(input.Key))
	info, err := fs.Stat(api.fsys, name)
	if err != nil {
		return nil, toS3NoSuchKeyIfNoExist(err)
	}
	if info.IsDir() {
		return nil, toS3NoSuchKeyIfNoExist(fs.ErrNotExist)
	}

	var in io.ReadCloser
	body := &lazyReadCloser{
		readFunc: func(p []byte) (int, error) {
			if in == nil {
				var err error
				in, err = api.fsys.Open(name)
				if err != nil {
					return 0, err
				}
			}
			return in.Read(p)
		},
		closeFunc: func() error {
			if in != nil {
				return in.Close()
			}
			return nil
		},
	}

	return &s3.GetObjectOutput{
		Body:          body,
		ContentLength: aws.Int64(info.Size()),
		LastModified:  aws.Time(info.ModTime()),
	}, nil
}

// PutObject API operation for the filesystem.
func (api *fsS3api) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	name := path.Join(aws.ToString(input.Bucket), aws.ToString(input.Key))
	output := &s3.PutObjectOutput{}
	f, err := wfs.CreateFile(api.fsys, name, fs.ModePerm)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, input.Body); err != nil {
		return nil, err
	}
	return output, nil
}

// CopyObject API operation for the filesystem.
func (api *fsS3api) CopyObject(_ context.Context, input *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	src := aws.ToString(input.CopySource)
	dst := path.Join(aws.ToString(input.Bucket), aws.ToString(input.Key))

	f, err := api.fsys.Open(src)
	if err != nil {
		return nil, toS3NoSuchKeyIfNoExist(err)
	}
	defer func() { _ = f.Close() }()

	w, err := wfs.CreateFile(api.fsys, dst, fs.ModePerm)
	if err != nil {
		return nil, err
	}
	defer func() { _ = w.Close() }()

	if _, err := io.Copy(w, f); err != nil {
		return nil, err
	}
	return &s3.CopyObjectOutput{}, nil
}

func (api *fsS3api) namePrefixes(dirPtr, prefixPtr *string) (string, string, error) {
	prefix := aws.ToString(prefixPtr)
	namePrefix := ""
	dirWithPrefix := path.Join(aws.ToString(dirPtr), prefix)
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
	dir := path.Join(aws.ToString(input.Bucket), prefix)
	entries, err := fs.ReadDir(api.fsys, dir)
	if err != nil {
		return nil, toS3NoSuchKeyIfNoExist(err)
	}

	output := &s3.ListObjectsV2Output{}
	limit := getMaxKeys(input.MaxKeys)
	after := aws.ToString(input.StartAfter)
	limited := false
	truncated := false

	for _, entry := range entries {
		name := path.Join(prefix, entry.Name())
		if !strings.HasPrefix(name, namePrefix) {
			continue
		}
		if entry.IsDir() {
			output.CommonPrefixes = append(output.CommonPrefixes, s3types.CommonPrefix{
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
			return nil, toS3NoSuchKeyIfNoExist(err)
		}
		output.Contents = append(output.Contents, s3types.Object{
			Key:          aws.String(name),
			Size:         aws.Int64(info.Size()),
			LastModified: aws.Time(info.ModTime()),
		})
		limited = (toInt32(len(output.Contents)) >= limit)
	}

	output.IsTruncated = aws.Bool(truncated)
	return output, nil
}

func (api *fsS3api) walkDir(input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	prefix, namePrefix, err := api.namePrefixes(input.Bucket, input.Prefix)
	if err != nil {
		return nil, err
	}
	root := path.Join(aws.ToString(input.Bucket), prefix)
	output := &s3.ListObjectsV2Output{}
	limit := getMaxKeys(input.MaxKeys)
	after := aws.ToString(input.StartAfter)
	limited := false
	truncated := false

	err = fs.WalkDir(api.fsys, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == root || !strings.HasPrefix(name, namePrefix) || d.IsDir() {
			return nil
		}
		name, err = filepath.Rel(aws.ToString(input.Bucket), name)
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
			return toS3NoSuchKeyIfNoExist(err)
		}
		output.Contents = append(output.Contents, s3types.Object{
			Key:          aws.String(name),
			Size:         aws.Int64(info.Size()),
			LastModified: aws.Time(info.ModTime()),
		})
		limited = (toInt32(len(output.Contents)) >= limit)
		return nil
	})
	if err != nil {
		return nil, err
	}

	output.IsTruncated = aws.Bool(truncated)
	return output, nil
}

// ListObjectsV2 API operation for the filesystem.
func (api *fsS3api) ListObjectsV2(_ context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if aws.ToString(input.Delimiter) == "/" {
		return api.readDir(input)
	}
	return api.walkDir(input)
}

// DeleteObject API operation for the filesystem.
func (api *fsS3api) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	name := path.Join(aws.ToString(input.Bucket), aws.ToString(input.Key))
	if err := wfs.RemoveFile(api.fsys, name); err != nil {
		return nil, toS3NoSuchKeyIfNoExist(err)
	}
	return &s3.DeleteObjectOutput{}, nil
}

// DeleteObjects API operation for the filesystem.
func (api *fsS3api) DeleteObjects(_ context.Context, input *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	for _, id := range input.Delete.Objects {
		name := path.Join(aws.ToString(input.Bucket), aws.ToString(id.Key))
		if err := wfs.RemoveFile(api.fsys, name); err != nil {
			return nil, toS3NoSuchKeyIfNoExist(err)
		}
	}
	return &s3.DeleteObjectsOutput{}, nil
}
