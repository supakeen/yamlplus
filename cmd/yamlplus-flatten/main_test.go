package main

import (
	"bytes"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"simple.yaml":          {Data: []byte("key: value\n")},
		"multidoc.yaml":        {Data: []byte("---\nfirst: 1\n---\nsecond: 2\n")},
		"invalid.yaml":         {Data: []byte("invalid: yaml: [\n")},
		"base.yaml":            {Data: []byte("network: &net\n  timeout: 30s\n  retries: 3\n")},
		"xref.yaml":            {Data: []byte("net: !xref \"base.yaml#net\"\n")},
		"dir/a.yaml":           {Data: []byte("a: 1\n")},
		"dir/b.yml":            {Data: []byte("b: 2\n")},
		"recursive/c.yaml":     {Data: []byte("c: 3\n")},
		"recursive/sub/d.yaml": {Data: []byte("d: 4\n")},
		"baddir/broken.yaml":   {Data: []byte("bad: yaml: [\n")},
	}
}

func TestRunNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{}, testFS(), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Usage:")
}

func TestRunTooManyArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"a.yaml", "b.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Usage:")
}

func TestRunBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--nonexistent"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 1, code)
}

func TestRunMissingInputFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"nonexistent.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "error reading input file")
}

func TestRunRegisterFileError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-f", "nonexistent.yaml", "simple.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "error registering file")
}

func TestRunRegisterDirectoryError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-d", "nonexistent", "simple.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "error registering directory")
}

func TestRunRegisterRecursivelyError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-r", "nonexistent", "simple.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "error registering recursively")
}

func TestRunDecodeError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"invalid.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "error decoding document")
}

func TestRunNoDocuments(t *testing.T) {
	fs := fstest.MapFS{
		"empty.yaml": {Data: []byte("")},
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"empty.yaml"}, fs, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "no documents found")
}

func TestRunSingleDocument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"simple.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "key: value")
	assert.NotContains(t, stdout.String(), "---")
}

func TestRunMultiDocument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"multidoc.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "first: 1")
	assert.Contains(t, stdout.String(), "---")
	assert.Contains(t, stdout.String(), "second: 2")
}

func TestRunWithXref(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-f", "base.yaml", "xref.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "timeout: 30s")
	assert.Contains(t, stdout.String(), "retries: 3")
}

func TestRunRegisterDirectory(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-d", "dir", "simple.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr.String())
}

func TestRunRegisterRecursively(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-r", "recursive", "simple.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr.String())
}

func TestRunRegisterDirectoryWithBadYAML(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-d", "baddir", "simple.yaml"}, testFS(), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "error registering directory")
}

func TestRunRegisterRecursivelyWithBadYAML(t *testing.T) {
	fs := fstest.MapFS{
		"simple.yaml":          {Data: []byte("key: value\n")},
		"rdir/ok.yaml":         {Data: []byte("ok: true\n")},
		"rdir/sub/broken.yaml": {Data: []byte("bad: yaml: [\n")},
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"-r", "rdir", "simple.yaml"}, fs, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "error registering recursively")
}

func TestRunLongFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--register-file", "base.yaml",
		"--register-directory", "dir",
		"--register-recursively", "recursive",
		"xref.yaml",
	}, testFS(), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr.String())
	assert.Contains(t, stdout.String(), "timeout: 30s")
}

func TestStringSlice(t *testing.T) {
	var s stringSlice
	assert.Equal(t, "", s.String())

	s.Set("a")
	s.Set("b")
	assert.Equal(t, "a, b", s.String())
}
