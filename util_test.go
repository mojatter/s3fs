package s3fs

import (
	"io/fs"
	"reflect"
	"testing"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestIsNotExist(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{
			err:  fs.ErrNotExist,
			want: true,
		}, {
			err:  &fs.PathError{Err: fs.ErrNotExist},
			want: true,
		}, {
			err:  fs.ErrExist,
			want: false,
		},
	}
	for _, test := range tests {
		got := isNotExist(test.err)
		if got != test.want {
			t.Errorf(`Error isNotExist(%v) returns %v; want %v`, test.err, got, test.want)
		}
	}
}

func TestIsS3NoSuchKey(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{
			err:  &s3types.NoSuchKey{},
			want: true,
		}, {
			err:  fs.ErrNotExist,
			want: false,
		},
	}
	for _, test := range tests {
		got := isS3NoSuchKey(test.err)
		if got != test.want {
			t.Errorf(`Error isS3NoSuchKey(%v) returns %v; want %v`, test.err, got, test.want)
		}
	}
}

func TestToPathError(t *testing.T) {
	op := "open"
	name := "test.txt"

	tests := []struct {
		err  error
		want error
	}{
		{
			err:  &s3types.NoSuchKey{},
			want: &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist},
		}, {
			err:  fs.ErrNotExist,
			want: &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist},
		}, {
			err:  fs.ErrExist,
			want: &fs.PathError{Op: op, Path: name, Err: fs.ErrExist},
		},
	}
	for _, test := range tests {
		got := toPathError(test.err, op, name)
		if got.Error() != test.want.Error() {
			t.Errorf(`Error toPathError(%v) returns %v; want %v`, test.err, got, test.want)
		}
	}
}

func TestToS3NoSuchKeyIfNoExist(t *testing.T) {
	tests := []struct {
		err  error
		want error
	}{
		{
			err:  fs.ErrNotExist,
			want: &s3types.NoSuchKey{},
		}, {
			err:  &fs.PathError{Err: fs.ErrNotExist},
			want: &s3types.NoSuchKey{},
		}, {
			err:  fs.ErrExist,
			want: fs.ErrExist,
		},
	}
	for _, test := range tests {
		got := toS3NoSuchKeyIfNoExist(test.err)
		if got.Error() != test.want.Error() {
			t.Errorf(`Error toS3NoSuchKeyIfNoExist(%v) returns %v; want %v`, test.err, got, test.want)
		}
	}
}

func TestNormalizePrefix(t *testing.T) {
	tests := []struct {
		prefix string
		want   string
	}{
		{
			prefix: ".",
			want:   "",
		}, {
			prefix: "/.",
			want:   "",
		}, {
			prefix: "dir",
			want:   "dir/",
		}, {
			prefix: "dir/",
			want:   "dir/",
		}, {
			prefix: "dir/.",
			want:   "dir/",
		},
	}
	for _, test := range tests {
		got := normalizePrefix(test.prefix)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf(`Error normalizePrefix(%s) returns %s; want %s`, test.prefix, got, test.want)
		}
	}
}

func TestNormalizePrefixPattern(t *testing.T) {
	tests := []struct {
		prefix  string
		pattern string
		want    string
	}{
		{
			prefix:  "prefix",
			pattern: "*",
			want:    "prefix/",
		}, {
			prefix:  "prefix",
			pattern: "d[i-i]r/",
			want:    "prefix/d",
		}, {
			prefix:  "prefix",
			pattern: "dir/",
			want:    "prefix/dir/",
		},
	}
	for _, test := range tests {
		got := normalizePrefixPattern(test.prefix, test.pattern)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf(`Error normalizePrefixPattern(%s, %s) returns %s; want %s`, test.prefix, test.pattern, got, test.want)
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		keys []string
		key  string
		want bool
	}{
		{
			keys: []string{"abc", "def", "ghi"},
			key:  "def",
			want: true,
		}, {
			keys: []string{"abc", "def", "ghi"},
			key:  "xyz",
			want: false,
		},
	}
	for _, test := range tests {
		got := contains(test.keys, test.key)
		if got != test.want {
			t.Errorf(`Error contains(%v, %v) returns %v; want %v`, test.keys, test.key, got, test.want)
		}
	}
}
