package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/supakeen/yamlplus"
	yaml "go.yaml.in/yaml/v3"
)

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.DirFS("."), os.Stdout, os.Stderr))
}

func run(args []string, fsys fs.FS, stdout, stderr io.Writer) int {
	var files, dirs, recursive stringSlice

	flags := flag.NewFlagSet("yamlplus-flatten", flag.ContinueOnError)
	flags.SetOutput(stderr)

	flags.Var(&files, "register-file", "register a single YAML file (repeatable)")
	flags.Var(&files, "f", "shorthand for --register-file")
	flags.Var(&dirs, "register-directory", "register all YAML files in a directory (repeatable)")
	flags.Var(&dirs, "d", "shorthand for --register-directory")
	flags.Var(&recursive, "register-recursively", "register all YAML files in a directory tree (repeatable)")
	flags.Var(&recursive, "r", "shorthand for --register-recursively")

	flags.Usage = func() {
		fmt.Fprintf(stderr, "Usage: yamlplus-flatten [flags] <input.yaml>\n\nFlatten a yamlplus YAML file by resolving all !xref tags.\n\nFlags:\n")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		return 1
	}

	if flags.NArg() != 1 {
		flags.Usage()
		return 1
	}

	inputPath := flags.Arg(0)

	loader := yamlplus.NewLoader(fsys)

	for _, f := range files {
		if err := loader.RegisterFile(f); err != nil {
			fmt.Fprintf(stderr, "error registering file %q: %v\n", f, err)
			return 1
		}
	}

	for _, d := range dirs {
		if err := loader.RegisterDirectory(d); err != nil {
			fmt.Fprintf(stderr, "error registering directory %q: %v\n", d, err)
			return 1
		}
	}

	for _, r := range recursive {
		if err := loader.RegisterRecursively(r); err != nil {
			fmt.Fprintf(stderr, "error registering recursively %q: %v\n", r, err)
			return 1
		}
	}

	data, err := fs.ReadFile(fsys, inputPath)
	if err != nil {
		fmt.Fprintf(stderr, "error reading input file: %v\n", err)
		return 1
	}

	dec := loader.NewDecoder(bytes.NewReader(data))

	docIndex := 0
	for {
		var doc any
		if err := dec.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(stderr, "error decoding document: %v\n", err)
			return 1
		}

		if docIndex > 0 {
			fmt.Fprintln(stdout, "---")
		}

		out, err := yaml.Marshal(doc)
		if err != nil {
			fmt.Fprintf(stderr, "error marshaling output: %v\n", err)
			return 1
		}

		stdout.Write(out)
		docIndex++
	}

	if docIndex == 0 {
		fmt.Fprintln(stderr, "error: no documents found in input file")
		return 1
	}

	return 0
}
