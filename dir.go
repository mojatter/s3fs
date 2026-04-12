package s3fs

import (
	"io"
	"io/fs"
	"sort"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type s3Dir struct {
	*content
	fsys   *S3FS
	prefix string
	after  string
	eof    bool
	cache  []fs.DirEntry
}

var _ fs.ReadDirFile = (*s3Dir)(nil)

func newS3Dir(fsys *S3FS, prefix string) *s3Dir {
	prefix = normalizePrefix(fsys.key(prefix))
	return &s3Dir{
		content: newDirContent(prefix),
		fsys:    fsys,
		prefix:  prefix,
	}
}

// Read reads bytes from this file.
func (d *s3Dir) Read(p []byte) (int, error) {
	return 0, &fs.PathError{Op: "Read", Path: d.prefix, Err: syscall.EISDIR}
}

// Stat returns the fs.FileInfo of this file.
func (d *s3Dir) Stat() (fs.FileInfo, error) {
	return d, nil
}

// Close closes streams.
func (d *s3Dir) Close() error {
	return nil
}

// ReadDir reads the contents of the directory and returns a slice of up to n
// DirEntry values in ascending sorted by filename.
func (d *s3Dir) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		return d.listAll()
	}
	return d.list(n)
}

func (d *s3Dir) listAll() ([]fs.DirEntry, error) {
	var allEntries []fs.DirEntry
	for len(d.cache) > 0 || !d.eof {
		entries, err := d.list(d.fsys.ListBufferSize)
		if err != nil && err != io.EOF {
			return nil, err
		}
		allEntries = append(allEntries, entries...)
	}
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].Name() < allEntries[j].Name()
	})
	return allEntries, nil
}

func (d *s3Dir) list(n int) ([]fs.DirEntry, error) {
	var entries []fs.DirEntry
	if cacheCount := len(d.cache); cacheCount > 0 {
		if n >= cacheCount {
			entries = d.cache
			d.cache = nil
		} else {
			entries = d.cache[0:n]
			d.cache = d.cache[n:]
		}
		n = n - cacheCount
		if d.eof || n <= 0 {
			return entries, nil
		}
	}

	if d.eof {
		return nil, io.EOF
	}
	api, err := d.fsys.client()
	if err != nil {
		return nil, err
	}
	input := &s3.ListObjectsV2Input{
		Bucket:     aws.String(d.fsys.bucket),
		Prefix:     aws.String(d.prefix),
		Delimiter:  aws.String("/"),
		MaxKeys:    aws.Int32(toInt32(n)),
		StartAfter: aws.String(d.after),
	}
	output, err := api.ListObjectsV2(d.fsys.Context(), input)
	if err != nil {
		return nil, err
	}

	for _, p := range output.CommonPrefixes {
		entries = append(entries, newDirContent(aws.ToString(p.Prefix)))
		d.after = aws.ToString(p.Prefix)
	}
	for _, o := range output.Contents {
		entries = append(entries, newFileContent(o))
		d.after = aws.ToString(o.Key)
	}
	d.eof = !aws.ToBool(output.IsTruncated)

	return entries, nil
}

// Open called by S3FS.Open(name string).
// Open calls d.list(n), if the results is empty then returns a PathError
// otherwise sets the results as d.cache.
func (d *s3Dir) open(n int) (*s3Dir, error) {
	entries, err := d.list(n)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, &fs.PathError{Op: "Open", Path: d.prefix, Err: fs.ErrNotExist}
	}
	d.cache = entries
	return d, nil
}
