package s3fs

import (
	"context"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/mojatter/wfs"
)



// S3API is the subset of the S3 client API used by this package.
// *s3.Client satisfies this interface.
type S3API interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
}

const (
	defaultDirOpenBufferSize = 100
	defaultListBufferSize    = 1000
)

// S3FS represents a filesystem on S3 (Amazon Simple Storage Service).
type S3FS struct {
	// DirOpenBufferSize is the buffer size for using objects as the directory. (Default 100)
	DirOpenBufferSize int
	// ListBufferSize is the buffer size for listing objects that is used on
	// ReadDir, Glob and RemoveAll. (Default 1000)
	ListBufferSize int
	api            S3API
	bucket         string
	dir            string
	ctx            context.Context
}

var (
	_ fs.FS            = (*S3FS)(nil)
	_ fs.GlobFS        = (*S3FS)(nil)
	_ fs.ReadDirFS     = (*S3FS)(nil)
	_ fs.ReadFileFS    = (*S3FS)(nil)
	_ fs.StatFS        = (*S3FS)(nil)
	_ fs.SubFS         = (*S3FS)(nil)
	_ wfs.WriteFileFS  = (*S3FS)(nil)
	_ wfs.RemoveFileFS = (*S3FS)(nil)
)

// New returns a filesystem for the tree of objects rooted at the specified bucket.
// The S3 client is lazily initialized on the first operation using the default
// AWS configuration. Use WithClient or WithConfig to provide a client explicitly.
func New(bucket string) *S3FS {
	return &S3FS{
		DirOpenBufferSize: defaultDirOpenBufferSize,
		ListBufferSize:    defaultListBufferSize,
		bucket:            bucket,
	}
}

// NewWithClient returns a filesystem for the tree of objects rooted at the
// specified bucket with the given S3 client.
//
// Example:
//
//	cfg, err := config.LoadDefaultConfig(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fsys := s3fs.NewWithClient(bucket, s3.NewFromConfig(cfg))
func NewWithClient(bucket string, client S3API) *S3FS {
	return New(bucket).WithClient(client)
}

// WithClient sets the S3 client to use for operations.
func (fsys *S3FS) WithClient(client S3API) *S3FS {
	fsys.api = client
	return fsys
}

// WithConfig sets the S3 client from the given AWS configuration.
func (fsys *S3FS) WithConfig(cfg aws.Config) *S3FS {
	fsys.api = s3.NewFromConfig(cfg)
	return fsys
}

// WithContext sets the context used for S3 operations and client initialization.
func (fsys *S3FS) WithContext(ctx context.Context) *S3FS {
	fsys.ctx = ctx
	return fsys
}

// Context returns the context for S3 operations.
// If no context has been set, it defaults to context.Background().
func (fsys *S3FS) Context() context.Context {
	if fsys.ctx == nil {
		fsys.ctx = context.Background()
	}
	return fsys.ctx
}

// client returns the S3 API client, lazily initializing it if necessary.
func (fsys *S3FS) client() (S3API, error) {
	if fsys.api == nil {
		cfg, err := awsconfig.LoadDefaultConfig(fsys.Context())
		if err != nil {
			return nil, err
		}
		fsys.api = s3.NewFromConfig(cfg)
	}
	return fsys.api, nil
}

func (fsys *S3FS) key(name string) string {
	return path.Clean(path.Join(fsys.dir, name))
}

func (fsys *S3FS) rel(name string) string {
	return strings.TrimPrefix(name, normalizePrefix(fsys.dir))
}

func (fsys *S3FS) openFile(name string) (*s3File, error) {
	if !fs.ValidPath(name) {
		return nil, toPathError(fs.ErrInvalid, "Open", name)
	}
	if name == "." || strings.HasSuffix(name, "/.") {
		return nil, toPathError(fs.ErrNotExist, "Open", name)
	}
	api, err := fsys.client()
	if err != nil {
		return nil, toPathError(err, "Open", name)
	}
	input := &s3.GetObjectInput{
		Bucket: aws.String(fsys.bucket),
		Key:    aws.String(fsys.key(name)),
	}
	output, err := api.GetObject(fsys.Context(), input)
	if err != nil {
		return nil, toPathError(err, "Open", name)
	}
	return newS3File(name, output), nil
}

// Open opens the named file or directory.
func (fsys *S3FS) Open(name string) (fs.File, error) {
	f, err := fsys.openFile(name)
	if err != nil && isNotExist(err) {
		return newS3Dir(fsys, name).open(fsys.DirOpenBufferSize)
	}
	return f, err
}

// ReadDir reads the named directory and returns a list of directory entries
// sorted by filename.
func (fsys *S3FS) ReadDir(dir string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(dir) {
		return nil, toPathError(fs.ErrInvalid, "ReadDir", dir)
	}
	return newS3Dir(fsys, dir).ReadDir(-1)
}

// ReadFile reads the named file and returns its contents.
func (fsys *S3FS) ReadFile(name string) ([]byte, error) {
	f, err := fsys.openFile(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	return io.ReadAll(f)
}

// Stat returns a FileInfo describing the file. If there is an error, it should be
// of type *PathError.
func (fsys *S3FS) Stat(name string) (fs.FileInfo, error) {
	f, err := fsys.openFile(name)
	if err != nil && isNotExist(err) {
		return newS3Dir(fsys, name).open(1)
	}
	return f, err
}

// Sub returns an FS corresponding to the subtree rooted at dir.
func (fsys *S3FS) Sub(dir string) (fs.FS, error) {
	if !fs.ValidPath(dir) {
		return nil, toPathError(fs.ErrInvalid, "Sub", dir)
	}
	subFsys := &S3FS{
		DirOpenBufferSize: fsys.DirOpenBufferSize,
		ListBufferSize:    fsys.ListBufferSize,
		api:               fsys.api,
		bucket:            fsys.bucket,
		dir:               path.Join(fsys.dir, dir),
		ctx:               fsys.ctx,
	}
	return subFsys, nil
}

// Glob returns the names of all files matching pattern, providing an implementation
// of the top-level Glob function.
func (fsys *S3FS) Glob(pattern string) ([]string, error) {
	if pattern == "" || pattern == "*" {
		entries, err := fsys.ReadDir("")
		if err != nil {
			return nil, err
		}
		var keys []string
		for _, entry := range entries {
			keys = append(keys, entry.Name())
		}
		return keys, nil
	}
	// NOTE: Validate pattern
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, toPathError(err, "Glob", pattern)
	}
	keys, err := fsys.glob([]string{""}, strings.Split(pattern, "/"), nil)
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, key := range keys {
		matches = appendIfMatch(matches, key, pattern)
	}
	sort.Strings(matches)
	return matches, nil
}

func (fsys *S3FS) glob(dirs, patterns []string, matches []string) ([]string, error) {
	dirOnly := len(patterns) > 1
	var subDirs []string
	for _, dir := range dirs {
		keys, err := fsys.listForGlob(path.Join(dir, patterns[0]), dirOnly)
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if dirOnly {
				subDirs = append(subDirs, key)
			}
			matches = append(matches, key)
		}
	}
	if len(subDirs) > 0 && dirOnly {
		return fsys.glob(subDirs, patterns[1:], matches)
	}
	return matches, nil
}

func (fsys *S3FS) listForGlob(pattern string, dirOnly bool) ([]string, error) {
	api, err := fsys.client()
	if err != nil {
		return nil, toPathError(err, "Glob", pattern)
	}
	input := &s3.ListObjectsV2Input{
		Bucket:    aws.String(fsys.bucket),
		Prefix:    aws.String(normalizePrefixPattern(fsys.dir, pattern)),
		MaxKeys:   aws.Int32(toInt32(fsys.ListBufferSize)),
		Delimiter: aws.String("/"),
	}
	var keys []string
	for {
		output, err := api.ListObjectsV2(fsys.Context(), input)
		if err != nil {
			return nil, toPathError(err, "Glob", pattern)
		}
		for _, p := range output.CommonPrefixes {
			key := strings.TrimRight(fsys.rel(aws.ToString(p.Prefix)), "/")
			keys = appendIfMatch(keys, key, pattern)
		}
		if dirOnly {
			return keys, nil
		}
		for _, o := range output.Contents {
			key := fsys.rel(aws.ToString(o.Key))
			keys = appendIfMatch(keys, key, pattern)
			input.StartAfter = o.Key
		}
		if !aws.ToBool(output.IsTruncated) {
			break
		}
	}
	return keys, nil
}

// MkdirAll always do nothing.
func (fsys *S3FS) MkdirAll(dir string, mode fs.FileMode) error {
	return nil
}

// CreateFile creates the named file.
// The specified mode is ignored.
func (fsys *S3FS) CreateFile(name string, mode fs.FileMode) (wfs.WriterFile, error) {
	if !fs.ValidPath(name) {
		return nil, toPathError(fs.ErrInvalid, "CreateFile", name)
	}

	if _, err := fsys.openFile(name); err != nil {
		if !isNotExist(err) {
			return nil, toPathError(err, "CreateFile", name)
		}
		if _, err := newS3Dir(fsys, name).open(1); err == nil {
			return nil, toPathError(syscall.EISDIR, "CreateFile", name)
		}
	}
	dir := path.Dir(name)
	if _, err := fsys.openFile(dir); err == nil {
		return nil, toPathError(syscall.ENOTDIR, "CreateFile", dir)
	}

	return newS3WriterFile(fsys, name), nil
}

// WriteFile writes the specified bytes to the named file.
// The specified mode is ignored.
func (fsys *S3FS) WriteFile(name string, p []byte, mode fs.FileMode) (int, error) {
	w, err := fsys.CreateFile(name, mode)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(p)
	if err != nil {
		return 0, toPathError(err, "Write", name)
	}
	return n, w.Close()
}

// RemoveFile removes the specified named file.
func (fsys *S3FS) RemoveFile(name string) error {
	api, err := fsys.client()
	if err != nil {
		return toPathError(err, "RemoveFile", name)
	}
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(fsys.bucket),
		Key:    aws.String(fsys.key(name)),
	}
	_, err = api.DeleteObject(fsys.Context(), input)
	if err != nil {
		return toPathError(err, "RemoveFile", name)
	}
	return nil
}

// RemoveAll removes path and any children it contains.
func (fsys *S3FS) RemoveAll(dir string) error {
	api, err := fsys.client()
	if err != nil {
		return toPathError(err, "RemoveAll", dir)
	}
	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(fsys.bucket),
		Prefix:  aws.String(normalizePrefix(fsys.key(dir))),
		MaxKeys: aws.Int32(toInt32(fsys.ListBufferSize)),
	}
	for {
		output, err := api.ListObjectsV2(fsys.Context(), input)
		if err != nil {
			return toPathError(err, "RemoveAll", dir)
		}
		var ids []s3types.ObjectIdentifier
		for _, o := range output.Contents {
			ids = append(ids, s3types.ObjectIdentifier{Key: o.Key})
			input.StartAfter = o.Key
		}

		_, err = api.DeleteObjects(fsys.Context(), &s3.DeleteObjectsInput{
			Bucket: aws.String(fsys.bucket),
			Delete: &s3types.Delete{Quiet: aws.Bool(true), Objects: ids},
		})
		if err != nil {
			return toPathError(err, "RemoveAll", dir)
		}

		if !aws.ToBool(output.IsTruncated) {
			break
		}
	}
	return nil
}
