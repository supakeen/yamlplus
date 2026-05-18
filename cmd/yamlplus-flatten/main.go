package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
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
	var files, dirs, recursive stringSlice

	flag.Var(&files, "register-file", "register a single YAML file (repeatable)")
	flag.Var(&files, "f", "shorthand for --register-file")
	flag.Var(&dirs, "register-directory", "register all YAML files in a directory (repeatable)")
	flag.Var(&dirs, "d", "shorthand for --register-directory")
	flag.Var(&recursive, "register-recursively", "register all YAML files in a directory tree (repeatable)")
	flag.Var(&recursive, "r", "shorthand for --register-recursively")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: yamlplus-flatten [flags] <input.yaml>\n\nFlatten a yamlplus YAML file by resolving all !xref tags.\n\nFlags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	inputPath := flag.Arg(0)

	loader := yamlplus.NewLoader(os.DirFS("."))

	for _, f := range files {
		if err := loader.RegisterFile(f); err != nil {
			fmt.Fprintf(os.Stderr, "error registering file %q: %v\n", f, err)
			os.Exit(1)
		}
	}

	for _, d := range dirs {
		if err := loader.RegisterDirectory(d); err != nil {
			fmt.Fprintf(os.Stderr, "error registering directory %q: %v\n", d, err)
			os.Exit(1)
		}
	}

	for _, r := range recursive {
		if err := loader.RegisterRecursively(r); err != nil {
			fmt.Fprintf(os.Stderr, "error registering recursively %q: %v\n", r, err)
			os.Exit(1)
		}
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading input file: %v\n", err)
		os.Exit(1)
	}

	dec := loader.NewDecoder(bytes.NewReader(data))

	docIndex := 0
	for {
		var doc any
		if err := dec.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "error decoding document: %v\n", err)
			os.Exit(1)
		}

		if docIndex > 0 {
			fmt.Println("---")
		}

		out, err := yaml.Marshal(doc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error marshaling output: %v\n", err)
			os.Exit(1)
		}

		os.Stdout.Write(out)
		docIndex++
	}

	if docIndex == 0 {
		fmt.Fprintln(os.Stderr, "error: no documents found in input file")
		os.Exit(1)
	}
}
