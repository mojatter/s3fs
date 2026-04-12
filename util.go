package s3fs

import (
	"errors"
	"io/fs"
	"math"
	"path"
	"strings"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// toInt32 safely converts an int to int32, capping at math.MaxInt32.
func toInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(n) //nolint:gosec // overflow is handled by the cap above
}

func isNotExist(err error) bool {
	if err == fs.ErrNotExist {
		return true
	}
	var pathErr *fs.PathError
	return errors.As(err, &pathErr) && pathErr.Err == fs.ErrNotExist
}

func isS3NoSuchKey(err error) bool {
	var noSuchKeyErr *s3types.NoSuchKey
	return errors.As(err, &noSuchKeyErr)
}

func toPathError(err error, op, name string) error {
	if isS3NoSuchKey(err) {
		err = fs.ErrNotExist
	}
	return &fs.PathError{Op: op, Path: name, Err: err}
}

func toS3NoSuchKeyIfNoExist(err error) error {
	if isNotExist(err) {
		return &s3types.NoSuchKey{}
	}
	return err
}

func normalizePrefix(prefix string) string {
	prefix = path.Clean(prefix)
	if prefix == "." || prefix == "/" {
		prefix = ""
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	return prefix
}

func normalizePrefixPattern(prefix, pattern string) string {
	prefix = normalizePrefix(prefix)
LOOP:
	for i, c := range pattern {
		switch c {
		case '*', '?', '[', '\\':
			pattern = pattern[:i]
			break LOOP
		}
	}
	joined := path.Join(prefix, pattern)
	if strings.HasSuffix(pattern, "/") || (joined != "" && pattern == "") {
		return joined + "/"
	}
	return joined
}

func contains(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

func appendIfMatch(keys []string, key, pattern string) []string {
	if ok, _ := path.Match(pattern, key); ok && !contains(keys, key) {
		keys = append(keys, key)
	}
	return keys
}
