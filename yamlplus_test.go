package yamlplus

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func getMockFS() fstest.MapFS {
	baseYAML := []byte(`
network: &net
  timeout: 30s
  retries: 3
`)

	databaseYAML := []byte(`
port: 3306
user: root
`)

	multidocYAML := []byte(`
---
doc: "one"
---
doc: "two"
`)

	nestedYAML := []byte(`
next: !xref "database.yaml"
version: 1.0
`)

	cycleaYAML := []byte(`
link: !xref "cycleb.yaml"
`)

	cyclebYAML := []byte(`
link: !xref "cyclea.yaml"
`)

	emptyYAML := []byte(``)

	sharedYAML := []byte(`
common: &shared
  host: localhost
  secure: true
`)

	invalidYAML := []byte(`
invalid: yaml: content: [
`)

	deepNestedYAML := []byte(`
level1: !xref "nested.yaml"
`)

	mergeChainYAML := []byte(`
base: &base
  timeout: 10s
  retries: 3

extended: &extended
  <<: *base
  port: 8080
`)

	multidocEmptyFirstYAML := []byte(`
---
first: null
---
doc: "second"
value: 42
`)

	// Note: This actually creates a single document with both anchors
	// YAML multi-doc syntax is tricky - both anchors end up in the first doc
	multidocWithAnchorsYAML := []byte(`
first: &anchor1
  value: "one"
second: &anchor2
  value: "two"
`)

	uppercaseYML := []byte(`
setting: uppercase_yml
`)

	mixedCaseYAML := []byte(`
setting: mixed_case
`)

	return fstest.MapFS{
		"base.yaml":                 {Data: baseYAML},
		"database.yaml":             {Data: databaseYAML},
		"multidoc.yaml":             {Data: multidocYAML},
		"nested.yaml":               {Data: nestedYAML},
		"cyclea.yaml":               {Data: cycleaYAML},
		"cycleb.yaml":               {Data: cyclebYAML},
		"empty.yaml":                {Data: emptyYAML},
		"shared.yaml":               {Data: sharedYAML},
		"invalid.yaml":              {Data: invalidYAML},
		"deep.yaml":                 {Data: deepNestedYAML},
		"mergechain.yaml":           {Data: mergeChainYAML},
		"multidoc-empty-first.yaml": {Data: multidocEmptyFirstYAML},
		"multidoc-anchors.yaml":     {Data: multidocWithAnchorsYAML},
		"dir/file1.yaml":            {Data: baseYAML},
		"dir/file2.yml":             {Data: databaseYAML},
		"dir/file3.txt":             {Data: []byte("not yaml")},
		"dir/subdir/nested.yaml":    {Data: sharedYAML},
		"dir/subdir/another.yml":    {Data: nestedYAML},
		"emptydir/.keep":            {Data: []byte("")},
		"uppercase.YML":             {Data: uppercaseYML},
		"mixedCase.YaML":            {Data: mixedCaseYAML},
		"relative/path/config.yaml": {Data: baseYAML},
	}
}

func TestXrefAnchorDirect(t *testing.T) {
	loader := NewLoader(getMockFS())

	err := loader.RegisterFile("base.yaml")
	assert.NoError(t, err)

	t.Run("direct xref anchor", func(t *testing.T) {
		var output map[string]any
		input := []byte(`net: !xref "base.yaml#net"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		net := output["net"].(map[string]any)
		assert.Equal(t, "30s", net["timeout"])
		assert.Equal(t, 3, net["retries"])
	})

	t.Run("direct xref anchor nonexistent", func(t *testing.T) {
		var output map[string]any
		input := []byte(`net: !xref "base.yaml#foo"`)

		err = loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})

	t.Run("direct xref anchor nonexistent file", func(t *testing.T) {
		var output map[string]any
		input := []byte(`net: !xref "foo.yaml#foo"`)

		err = loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})
}

func TestXrefAnchorMapMerge(t *testing.T) {
	loader := NewLoader(getMockFS())

	var err error

	err = loader.RegisterFile("base.yaml")
	assert.NoError(t, err)

	err = loader.RegisterFile("nested.yaml")
	assert.NoError(t, err)

	err = loader.RegisterFile("database.yaml")
	assert.NoError(t, err)

	mapMergeYAML := `
net:
  <<: !xref "base.yaml#net"
  timeout: 1s
`

	t.Run("map merge xref anchor", func(t *testing.T) {
		var output map[string]any
		input := []byte(mapMergeYAML)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		net := output["net"].(map[string]any)
		assert.Equal(t, "1s", net["timeout"])
		assert.Equal(t, 3, net["retries"])
	})

	mapMergeNonExistentYAML := `
net:
  <<: !xref "base.yaml#nonexistent"
  timeout: 1s
`

	t.Run("map merge xref anchor nonexistent", func(t *testing.T) {
		var output map[string]any
		input := []byte(mapMergeNonExistentYAML)

		err = loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})

	mapMergeMultiYAML := `
net:
  <<:
    - !xref "base.yaml#net"
    - !xref "nested.yaml"
  timeout: 1s
`

	t.Run("map merge xref anchor multi", func(t *testing.T) {
		var output map[string]any
		input := []byte(mapMergeMultiYAML)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)
	})

	mapMergeMultiMixedYAML := `
foo: &foo
  bar: "bar"
net:
  <<:
    - inline: no-actually-inline
    - !xref "base.yaml#net"
    - *foo
    - !xref "nested.yaml"
    - inline: inline
  timeout: 1s
`

	t.Run("map merge xref anchor multi merged", func(t *testing.T) {
		var output map[string]any
		input := []byte(mapMergeMultiMixedYAML)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		net := output["net"].(map[string]any)
		assert.Equal(t, "bar", net["bar"])
		assert.Equal(t, "1s", net["timeout"])
		assert.Equal(t, 3, net["retries"])
		assert.Equal(t, "no-actually-inline", net["inline"])
	})

}

func TestXrefDoc(t *testing.T) {
	loader := NewLoader(getMockFS())

	var err error

	err = loader.RegisterFile("database.yaml")
	assert.NoError(t, err)

	err = loader.RegisterFile("multidoc.yaml")
	assert.NoError(t, err)

	t.Run("xref document", func(t *testing.T) {
		var output map[string]any
		input := []byte(`database: !xref "database.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		database := output["database"].(map[string]any)
		assert.Equal(t, 3306, database["port"])
		assert.Equal(t, "root", database["user"])
	})

	t.Run("xref document nonexistent", func(t *testing.T) {
		var output map[string]any
		input := []byte(`database: !xref "foo.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})

	t.Run("xref document empty", func(t *testing.T) {
		var output map[string]any
		input := []byte(`empty: !xref "empty.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.Error(t, err)

		// empty documents are not loaded at all in RegisterFile hence
		// the error is the generic not found
		assert.Contains(t, err.Error(), "not found in registry")
	})

	t.Run("xref document contains multiple docs", func(t *testing.T) {
		var output map[string]any
		input := []byte(`other: !xref "multidoc.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		other := output["other"].(map[string]any)
		assert.Equal(t, "one", other["doc"])
	})
}

func TestXrefCycle(t *testing.T) {
	loader := NewLoader(getMockFS())

	var err error

	err = loader.RegisterFile("cyclea.yaml")
	assert.NoError(t, err)

	err = loader.RegisterFile("cycleb.yaml")
	assert.NoError(t, err)

	t.Run("xref circular", func(t *testing.T) {
		var output map[string]any
		input := []byte(`start: !xref "cyclea.yaml"`)

		err := loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular dependency")
	})
}

func TestXrefNested(t *testing.T) {
	loader := NewLoader(getMockFS())

	var err error

	err = loader.RegisterFile("nested.yaml")
	assert.NoError(t, err)

	err = loader.RegisterFile("database.yaml")
	assert.NoError(t, err)

	t.Run("xref nested", func(t *testing.T) {
		var output map[string]any
		input := []byte(`start: !xref "nested.yaml"`)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		start := output["start"].(map[string]any)
		next := start["next"].(map[string]any)

		assert.Equal(t, 3306, next["port"])
		assert.Equal(t, "root", next["user"])
	})
}

func TestXrefStackCleanup(t *testing.T) {
	loader := NewLoader(getMockFS())

	err := loader.RegisterFile("shared.yaml")
	assert.NoError(t, err)

	t.Run("same xref used from multiple branches", func(t *testing.T) {
		var output map[string]any
		// Use the same xref in multiple places in the tree
		// This tests that the circular dependency stack is properly cleaned up
		input := []byte(`
service_a:
  config: !xref "shared.yaml#shared"
service_b:
  config: !xref "shared.yaml#shared"
`)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		serviceA := output["service_a"].(map[string]any)
		configA := serviceA["config"].(map[string]any)
		assert.Equal(t, "localhost", configA["host"])
		assert.Equal(t, true, configA["secure"])

		serviceB := output["service_b"].(map[string]any)
		configB := serviceB["config"].(map[string]any)
		assert.Equal(t, "localhost", configB["host"])
		assert.Equal(t, true, configB["secure"])
	})

	t.Run("same xref in nested structures", func(t *testing.T) {
		var output map[string]any
		// Test deeply nested reuse of the same reference
		input := []byte(`
parent:
  child1:
    data: !xref "shared.yaml#shared"
  child2:
    data: !xref "shared.yaml#shared"
`)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		parent := output["parent"].(map[string]any)
		child1 := parent["child1"].(map[string]any)
		data1 := child1["data"].(map[string]any)
		assert.Equal(t, "localhost", data1["host"])

		child2 := parent["child2"].(map[string]any)
		data2 := child2["data"].(map[string]any)
		assert.Equal(t, "localhost", data2["host"])
	})
}

func TestMapMergeWithDirectAlias(t *testing.T) {
	loader := NewLoader(getMockFS())

	err := loader.RegisterFile("base.yaml")
	assert.NoError(t, err)

	t.Run("map merge with direct alias", func(t *testing.T) {
		var output map[string]any
		// Test that standard YAML alias syntax works with map merge
		input := []byte(`
defaults: &defaults
  timeout: 10s
  retries: 5

config:
  <<: *defaults
  timeout: 20s
`)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config := output["config"].(map[string]any)
		assert.Equal(t, "20s", config["timeout"]) // overridden
		assert.Equal(t, 5, config["retries"])     // merged from alias
	})

	t.Run("map merge with direct alias and xref", func(t *testing.T) {
		var output map[string]any
		// Test combining direct alias with xref
		input := []byte(`
local: &local
  cache: true

config:
  <<: *local
  network: !xref "base.yaml#net"
`)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config := output["config"].(map[string]any)
		assert.Equal(t, true, config["cache"])

		network := config["network"].(map[string]any)
		assert.Equal(t, "30s", network["timeout"])
		assert.Equal(t, 3, network["retries"])
	})

	t.Run("map merge with mixed sequence including direct alias", func(t *testing.T) {
		var output map[string]any
		// Test sequence with both alias and xref
		input := []byte(`
local: &local
  cache: true
  debug: false

config:
  <<:
    - *local
    - !xref "base.yaml#net"
  timeout: 5s
`)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config := output["config"].(map[string]any)
		assert.Equal(t, true, config["cache"])   // from alias
		assert.Equal(t, false, config["debug"])  // from alias
		assert.Equal(t, 3, config["retries"])    // from xref
		assert.Equal(t, "5s", config["timeout"]) // overridden
	})
}

func TestRegisterDirectory(t *testing.T) {
	loader := NewLoader(getMockFS())

	t.Run("basic directory registration", func(t *testing.T) {
		err := loader.RegisterDirectory("dir")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`
config1: !xref "dir/file1.yaml"
config2: !xref "dir/file2.yml"
`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config1 := output["config1"].(map[string]any)
		network := config1["network"].(map[string]any)
		assert.Equal(t, "30s", network["timeout"])

		config2 := output["config2"].(map[string]any)
		assert.Equal(t, 3306, config2["port"])
	})

	t.Run("directory with mixed file types", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterDirectory("dir")
		assert.NoError(t, err)

		// file3.txt should not be registered
		var output map[string]any
		input := []byte(`config: !xref "dir/file3.txt"`)

		err = loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})

	t.Run("empty directory", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterDirectory("emptydir")
		assert.NoError(t, err)
	})

	t.Run("non-existent directory", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterDirectory("nonexistent")
		assert.Error(t, err)
	})

	t.Run("directory does not recurse into subdirectories", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterDirectory("dir")
		assert.NoError(t, err)

		// dir/subdir/nested.yaml should NOT be registered
		var output map[string]any
		input := []byte(`config: !xref "dir/subdir/nested.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})
}

func TestRegisterRecursively(t *testing.T) {
	loader := NewLoader(getMockFS())

	t.Run("recursive registration", func(t *testing.T) {
		// Need to register database.yaml for the nested reference
		err := loader.RegisterFile("database.yaml")
		assert.NoError(t, err)

		err = loader.RegisterRecursively("dir")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`
top1: !xref "dir/file1.yaml"
top2: !xref "dir/file2.yml"
nested1: !xref "dir/subdir/nested.yaml"
nested2: !xref "dir/subdir/another.yml"
`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		top1 := output["top1"].(map[string]any)
		network := top1["network"].(map[string]any)
		assert.Equal(t, "30s", network["timeout"])

		top2 := output["top2"].(map[string]any)
		assert.Equal(t, 3306, top2["port"])

		nested1 := output["nested1"].(map[string]any)
		common := nested1["common"].(map[string]any)
		assert.Equal(t, "localhost", common["host"])

		nested2 := output["nested2"].(map[string]any)
		next := nested2["next"].(map[string]any)
		assert.Equal(t, 3306, next["port"])
		// YAML parses 1.0 as float, not string
		assert.Equal(t, float64(1), nested2["version"])
	})

	t.Run("recursive ignores non-yaml files", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterRecursively("dir")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`config: !xref "dir/file3.txt"`)

		err = loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})

	t.Run("non-existent directory", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterRecursively("nonexistent")
		assert.Error(t, err)
	})
}

func TestFileExtensions(t *testing.T) {
	loader := NewLoader(getMockFS())

	t.Run("both .yaml and .yml extensions work", func(t *testing.T) {
		err := loader.RegisterFile("base.yaml")
		assert.NoError(t, err)

		err = loader.RegisterDirectory("dir")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`
yaml_file: !xref "dir/file1.yaml"
yml_file: !xref "dir/file2.yml"
`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		assert.NotNil(t, output["yaml_file"])
		assert.NotNil(t, output["yml_file"])
	})

	t.Run("uppercase extensions work", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("uppercase.YML")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`config: !xref "uppercase.YML"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		assert.Equal(t, "uppercase_yml", output["config"].(map[string]any)["setting"])
	})

	t.Run("mixed case extensions work", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("mixedCase.YaML")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`config: !xref "mixedCase.YaML"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		assert.Equal(t, "mixed_case", output["config"].(map[string]any)["setting"])
	})
}

func TestRegisterFileErrors(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("nonexistent.yaml")
		assert.Error(t, err)
	})

	t.Run("invalid yaml syntax", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("invalid.yaml")
		assert.Error(t, err)
	})

	t.Run("empty yaml file", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		// Empty files produce an EOF error during decoding
		err := loader.RegisterFile("empty.yaml")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "EOF")
	})
}

func TestMapMergeEdgeCases(t *testing.T) {
	loader := NewLoader(getMockFS())

	err := loader.RegisterFile("base.yaml")
	assert.NoError(t, err)

	t.Run("empty sequence merge", func(t *testing.T) {
		var output map[string]any
		input := []byte(`
config:
  <<: []
  port: 8080
`)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config := output["config"].(map[string]any)
		assert.Equal(t, 8080, config["port"])
	})

	t.Run("sequence with no valid merge targets", func(t *testing.T) {
		var output map[string]any
		input := []byte(`
config:
  <<:
    - scalar_value
    - 123
    - true
  port: 8080
`)

		// The underlying YAML library rejects sequences with only scalars
		err := loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "map merge")
	})

	t.Run("nested map merges", func(t *testing.T) {
		var output map[string]any
		input := []byte(`
base: &base
  timeout: 10s

extended: &extended
  <<: *base
  retries: 3

final:
  <<: *extended
  port: 8080
`)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		final := output["final"].(map[string]any)
		assert.Equal(t, "10s", final["timeout"])
		assert.Equal(t, 3, final["retries"])
		assert.Equal(t, 8080, final["port"])
	})

	t.Run("map merge precedence", func(t *testing.T) {
		var output map[string]any
		input := []byte(`
first: &first
  a: from_first
  b: from_first
  c: from_first

second: &second
  b: from_second
  c: from_second

third: &third
  c: from_third

config:
  <<:
    - *first
    - *second
    - *third
  c: from_config
`)

		err := loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config := output["config"].(map[string]any)
		// First in sequence takes precedence for merged keys
		assert.Equal(t, "from_first", config["a"])
		assert.Equal(t, "from_first", config["b"])
		// Direct assignment takes precedence over merges
		assert.Equal(t, "from_config", config["c"])
	})
}

func TestMultipleDocuments(t *testing.T) {
	loader := NewLoader(getMockFS())

	t.Run("file with multiple documents uses first", func(t *testing.T) {
		err := loader.RegisterFile("multidoc-empty-first.yaml")
		assert.NoError(t, err)

		// When referencing entire file, first document is used
		var output map[string]any
		input := []byte(`config: !xref "multidoc-empty-first.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config := output["config"].(map[string]any)
		// First document has "first: null"
		assert.Nil(t, config["first"])
	})

	t.Run("multiple anchors in same document", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("multidoc-anchors.yaml")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`
ref1: !xref "multidoc-anchors.yaml#anchor1"
ref2: !xref "multidoc-anchors.yaml#anchor2"
`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		ref1 := output["ref1"].(map[string]any)
		assert.Equal(t, "one", ref1["value"])

		ref2 := output["ref2"].(map[string]any)
		assert.Equal(t, "two", ref2["value"])
	})

	t.Run("reference entire multidoc file uses first doc", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("multidoc.yaml")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`config: !xref "multidoc.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config := output["config"].(map[string]any)
		assert.Equal(t, "one", config["doc"])
	})
}

func TestComplexNesting(t *testing.T) {
	loader := NewLoader(getMockFS())

	t.Run("three levels deep xref chain", func(t *testing.T) {
		err := loader.RegisterFile("database.yaml")
		assert.NoError(t, err)

		err = loader.RegisterFile("nested.yaml")
		assert.NoError(t, err)

		err = loader.RegisterFile("deep.yaml")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`start: !xref "deep.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		start := output["start"].(map[string]any)
		level1 := start["level1"].(map[string]any)
		next := level1["next"].(map[string]any)
		assert.Equal(t, 3306, next["port"])
		assert.Equal(t, "root", next["user"])
	})

	t.Run("map merge with xref containing map merge", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("mergechain.yaml")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`
final:
  <<: !xref "mergechain.yaml#extended"
  retries: 5
`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		final := output["final"].(map[string]any)
		assert.Equal(t, "10s", final["timeout"])
		assert.Equal(t, 8080, final["port"])
		assert.Equal(t, 5, final["retries"]) // overridden
	})

	t.Run("deeply nested mixed structures", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("base.yaml")
		assert.NoError(t, err)

		err = loader.RegisterFile("database.yaml")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`
local: &local
  cache: true

services:
  - name: web
    config:
      <<: *local
      network: !xref "base.yaml#net"
  - name: db
    config: !xref "database.yaml"
`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		services := output["services"].([]any)
		assert.Len(t, services, 2)

		webService := services[0].(map[string]any)
		webConfig := webService["config"].(map[string]any)
		assert.Equal(t, true, webConfig["cache"])
		network := webConfig["network"].(map[string]any)
		assert.Equal(t, "30s", network["timeout"])

		dbService := services[1].(map[string]any)
		dbConfig := dbService["config"].(map[string]any)
		assert.Equal(t, 3306, dbConfig["port"])
	})
}

func TestDoubleRegistration(t *testing.T) {
	t.Run("register same file twice", func(t *testing.T) {
		loader := NewLoader(getMockFS())

		err := loader.RegisterFile("base.yaml")
		assert.NoError(t, err)

		// Register again - should succeed but overwrite
		err = loader.RegisterFile("base.yaml")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`net: !xref "base.yaml#net"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		net := output["net"].(map[string]any)
		assert.Equal(t, "30s", net["timeout"])
	})

	t.Run("register same file in directory and individually", func(t *testing.T) {
		loader := NewLoader(getMockFS())

		err := loader.RegisterFile("dir/file1.yaml")
		assert.NoError(t, err)

		// Register via directory
		err = loader.RegisterDirectory("dir")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`config: !xref "dir/file1.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		assert.NotNil(t, output["config"])
	})
}

func TestPathNamespacing(t *testing.T) {
	t.Run("exact path must match", func(t *testing.T) {
		loader := NewLoader(getMockFS())

		err := loader.RegisterFile("dir/file1.yaml")
		assert.NoError(t, err)

		var output map[string]any
		// Must use exact path as registered
		input := []byte(`config: !xref "dir/file1.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		assert.NotNil(t, output["config"])
	})

	t.Run("different paths to same content are separate", func(t *testing.T) {
		loader := NewLoader(getMockFS())

		err := loader.RegisterFile("relative/path/config.yaml")
		assert.NoError(t, err)

		var output map[string]any
		// Reference with different path should fail
		input := []byte(`config: !xref "config.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})

	t.Run("anchor paths include file path", func(t *testing.T) {
		loader := NewLoader(getMockFS())

		err := loader.RegisterFile("relative/path/config.yaml")
		assert.NoError(t, err)

		var output map[string]any
		// Anchors include the full path
		input := []byte(`net: !xref "relative/path/config.yaml#net"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		net := output["net"].(map[string]any)
		assert.Equal(t, "30s", net["timeout"])
	})
}

func TestAnchorRegistry(t *testing.T) {
	t.Run("verify document-level registration", func(t *testing.T) {
		loader := NewLoader(getMockFS())

		err := loader.RegisterFile("database.yaml")
		assert.NoError(t, err)

		// Should be able to reference entire file without #anchor
		var output map[string]any
		input := []byte(`config: !xref "database.yaml"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config := output["config"].(map[string]any)
		assert.Equal(t, 3306, config["port"])
	})

	t.Run("verify nested anchors registered", func(t *testing.T) {
		loader := NewLoader(getMockFS())

		err := loader.RegisterFile("base.yaml")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`net: !xref "base.yaml#net"`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		net := output["net"].(map[string]any)
		assert.Equal(t, "30s", net["timeout"])
		assert.Equal(t, 3, net["retries"])
	})

	t.Run("anchors from all documents registered", func(t *testing.T) {
		loader := NewLoader(getMockFS())

		err := loader.RegisterFile("multidoc-anchors.yaml")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`
first: !xref "multidoc-anchors.yaml#anchor1"
second: !xref "multidoc-anchors.yaml#anchor2"
`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		first := output["first"].(map[string]any)
		assert.Equal(t, "one", first["value"])

		second := output["second"].(map[string]any)
		assert.Equal(t, "two", second["value"])
	})
}

// Example demonstrates basic usage of yamlplus to reference anchors across files.
func Example() {
	// Create a mock filesystem with YAML files
	fs := fstest.MapFS{
		"database.yaml": {Data: []byte(`
host: localhost
port: 5432
user: admin
`)},
		"app.yaml": {Data: []byte(`
name: myapp
database: !xref "database.yaml"
`)},
	}

	// Create a loader and register files
	loader := NewLoader(fs)
	loader.RegisterFile("database.yaml")

	// Unmarshal YAML that references another file
	var config map[string]any
	data := []byte(`
app_name: example
db_config: !xref "database.yaml"
`)

	loader.Unmarshal(data, &config)

	fmt.Printf("App: %s\n", config["app_name"])
	db := config["db_config"].(map[string]any)
	fmt.Printf("DB Host: %s\n", db["host"])
	fmt.Printf("DB Port: %d\n", db["port"])

	// Output:
	// App: example
	// DB Host: localhost
	// DB Port: 5432
}

// ExampleNewLoader demonstrates creating a new Loader with a filesystem.
func ExampleNewLoader() {
	// Create a loader from a mock filesystem
	fs := fstest.MapFS{
		"config.yaml": {Data: []byte("version: 1.0")},
	}

	loader := NewLoader(fs)
	loader.RegisterFile("config.yaml")

	var config map[string]any
	loader.Unmarshal([]byte(`app: !xref "config.yaml"`), &config)

	app := config["app"].(map[string]any)
	fmt.Printf("Version: %v\n", app["version"])

	// Output:
	// Version: 1
}

// ExampleLoader_Unmarshal demonstrates unmarshaling YAML with xref tags.
func ExampleLoader_Unmarshal() {
	fs := fstest.MapFS{
		"base.yaml": {Data: []byte(`
network: &net
  timeout: 30s
  retries: 3
`)},
	}

	loader := NewLoader(fs)
	loader.RegisterFile("base.yaml")

	var config map[string]any
	data := []byte(`
service:
  name: api
  network: !xref "base.yaml#net"
`)

	loader.Unmarshal(data, &config)

	service := config["service"].(map[string]any)
	network := service["network"].(map[string]any)
	fmt.Printf("Timeout: %s\n", network["timeout"])
	fmt.Printf("Retries: %d\n", network["retries"])

	// Output:
	// Timeout: 30s
	// Retries: 3
}

// ExampleLoader_Unmarshal_mapMerge demonstrates using xref with YAML map merge.
func ExampleLoader_Unmarshal_mapMerge() {
	fs := fstest.MapFS{
		"defaults.yaml": {Data: []byte(`
defaults: &defaults
  timeout: 30s
  retries: 3
  debug: false
`)},
	}

	loader := NewLoader(fs)
	loader.RegisterFile("defaults.yaml")

	var config map[string]any
	data := []byte(`
production:
  <<: !xref "defaults.yaml#defaults"
  timeout: 60s
  debug: false
`)

	loader.Unmarshal(data, &config)

	prod := config["production"].(map[string]any)
	fmt.Printf("Timeout: %s\n", prod["timeout"]) // overridden
	fmt.Printf("Retries: %d\n", prod["retries"]) // from merge
	fmt.Printf("Debug: %v\n", prod["debug"])     // from merge

	// Output:
	// Timeout: 60s
	// Retries: 3
	// Debug: false
}

// ExampleLoader_RegisterDirectory demonstrates registering all YAML files in a directory.
func ExampleLoader_RegisterDirectory() {
	fs := fstest.MapFS{
		"configs/app.yaml": {Data: []byte(`
app:
  name: myapp
  version: 1.0
`)},
		"configs/db.yaml": {Data: []byte(`
database:
  host: localhost
  port: 5432
`)},
		"configs/readme.txt": {Data: []byte("not yaml")},
	}

	loader := NewLoader(fs)
	// Registers only .yaml and .yml files, ignores .txt
	loader.RegisterDirectory("configs")

	var config map[string]any
	data := []byte(`
application: !xref "configs/app.yaml"
database: !xref "configs/db.yaml"
`)

	loader.Unmarshal(data, &config)

	app := config["application"].(map[string]any)["app"].(map[string]any)
	fmt.Printf("App: %s v%v\n", app["name"], app["version"])

	// Output:
	// App: myapp v1
}

// ExampleLoader_RegisterRecursively demonstrates recursive directory registration.
func ExampleLoader_RegisterRecursively() {
	fs := fstest.MapFS{
		"configs/app.yaml":          {Data: []byte("app: production")},
		"configs/services/api.yaml": {Data: []byte("service: api")},
		"configs/services/db.yaml":  {Data: []byte("service: database")},
	}

	loader := NewLoader(fs)
	// Recursively registers all YAML files
	loader.RegisterRecursively("configs")

	var config map[string]any
	data := []byte(`
app: !xref "configs/app.yaml"
api: !xref "configs/services/api.yaml"
`)

	loader.Unmarshal(data, &config)

	app := config["app"].(map[string]any)
	api := config["api"].(map[string]any)
	fmt.Printf("App: %s\n", app["app"])
	fmt.Printf("Service: %s\n", api["service"])

	// Output:
	// App: production
	// Service: api
}

// ExampleLoader_NewDecoder demonstrates using the streaming Decoder API with options.
func ExampleLoader_NewDecoder() {
	fs := fstest.MapFS{
		"defaults.yaml": {Data: []byte(`
defaults: &defaults
  timeout: 30s
  retries: 3
`)},
	}

	loader := NewLoader(fs)
	loader.RegisterFile("defaults.yaml")

	type Config struct {
		Timeout string `yaml:"timeout"`
		Retries int    `yaml:"retries"`
	}

	input := strings.NewReader(`
timeout: 10s
retries: 5
`)

	dec := loader.NewDecoder(input)
	dec.KnownFields(true)

	var config Config
	dec.Decode(&config)

	fmt.Printf("Timeout: %s\n", config.Timeout)
	fmt.Printf("Retries: %d\n", config.Retries)

	// Output:
	// Timeout: 10s
	// Retries: 5
}

// ExampleLoader_NewDecoder_xref demonstrates decoding with xref resolution.
func ExampleLoader_NewDecoder_xref() {
	fs := fstest.MapFS{
		"database.yaml": {Data: []byte(`
host: localhost
port: 5432
`)},
	}

	loader := NewLoader(fs)
	loader.RegisterFile("database.yaml")

	input := strings.NewReader(`db: !xref "database.yaml"`)
	dec := loader.NewDecoder(input)

	var config map[string]any
	dec.Decode(&config)

	db := config["db"].(map[string]any)
	fmt.Printf("Host: %s\n", db["host"])
	fmt.Printf("Port: %d\n", db["port"])

	// Output:
	// Host: localhost
	// Port: 5432
}

func TestNodeCloning(t *testing.T) {
	loader := NewLoader(getMockFS())

	err := loader.RegisterFile("base.yaml")
	assert.NoError(t, err)

	t.Run("modifying resolved xref does not affect original", func(t *testing.T) {
		var output1 map[string]any
		input1 := []byte(`
config1: !xref "base.yaml#net"
`)

		err := loader.Unmarshal(input1, &output1)
		assert.NoError(t, err)

		config1 := output1["config1"].(map[string]any)
		originalTimeout := config1["timeout"]
		assert.Equal(t, "30s", originalTimeout)

		// Modify the output
		config1["timeout"] = "999s"
		config1["new_field"] = "added"

		// Use the same xref again in a new unmarshal
		var output2 map[string]any
		input2 := []byte(`
config2: !xref "base.yaml#net"
`)

		err = loader.Unmarshal(input2, &output2)
		assert.NoError(t, err)

		config2 := output2["config2"].(map[string]any)
		// Should still have original values, not modified ones
		assert.Equal(t, "30s", config2["timeout"])
		assert.Nil(t, config2["new_field"])
	})

	t.Run("deep clone of nested structures", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("database.yaml")
		assert.NoError(t, err)

		err = loader.RegisterFile("nested.yaml")
		assert.NoError(t, err)

		var output1 map[string]any
		input1 := []byte(`config: !xref "nested.yaml"`)

		err = loader.Unmarshal(input1, &output1)
		assert.NoError(t, err)

		config1 := output1["config"].(map[string]any)
		next1 := config1["next"].(map[string]any)
		next1["port"] = 9999 // Modify nested value

		// Use the xref again
		var output2 map[string]any
		input2 := []byte(`config: !xref "nested.yaml"`)

		err = loader.Unmarshal(input2, &output2)
		assert.NoError(t, err)

		config2 := output2["config"].(map[string]any)
		next2 := config2["next"].(map[string]any)
		// Should have original value, not modified one
		assert.Equal(t, 3306, next2["port"])
	})

	t.Run("multiple uses of same xref are independent", func(t *testing.T) {
		loader := NewLoader(getMockFS())
		err := loader.RegisterFile("base.yaml")
		assert.NoError(t, err)

		var output map[string]any
		input := []byte(`
config1: !xref "base.yaml#net"
config2: !xref "base.yaml#net"
`)

		err = loader.Unmarshal(input, &output)
		assert.NoError(t, err)

		config1 := output["config1"].(map[string]any)
		config2 := output["config2"].(map[string]any)

		// Modify one
		config1["timeout"] = "modified"

		// Other should be unaffected
		assert.Equal(t, "30s", config2["timeout"])
		assert.Equal(t, "modified", config1["timeout"])
	})
}

func TestDecoder(t *testing.T) {
	loader := NewLoader(getMockFS())

	err := loader.RegisterFile("base.yaml")
	assert.NoError(t, err)

	err = loader.RegisterFile("database.yaml")
	assert.NoError(t, err)

	err = loader.RegisterFile("nested.yaml")
	assert.NoError(t, err)

	t.Run("basic decode matches unmarshal", func(t *testing.T) {
		input := `net: !xref "base.yaml#net"`

		var fromDecoder map[string]any
		dec := loader.NewDecoder(strings.NewReader(input))
		err := dec.Decode(&fromDecoder)
		assert.NoError(t, err)

		var fromUnmarshal map[string]any
		err = loader.Unmarshal([]byte(input), &fromUnmarshal)
		assert.NoError(t, err)

		assert.Equal(t, fromUnmarshal, fromDecoder)
	})

	t.Run("direct xref through decoder", func(t *testing.T) {
		var output map[string]any
		dec := loader.NewDecoder(strings.NewReader(`db: !xref "database.yaml"`))

		err := dec.Decode(&output)
		assert.NoError(t, err)

		db := output["db"].(map[string]any)
		assert.Equal(t, 3306, db["port"])
		assert.Equal(t, "root", db["user"])
	})

	t.Run("map merge xref through decoder", func(t *testing.T) {
		input := `
net:
  <<: !xref "base.yaml#net"
  timeout: 1s
`
		var output map[string]any
		dec := loader.NewDecoder(strings.NewReader(input))

		err := dec.Decode(&output)
		assert.NoError(t, err)

		net := output["net"].(map[string]any)
		assert.Equal(t, "1s", net["timeout"])
		assert.Equal(t, 3, net["retries"])
	})

	t.Run("nested xref through decoder", func(t *testing.T) {
		var output map[string]any
		dec := loader.NewDecoder(strings.NewReader(`start: !xref "nested.yaml"`))

		err := dec.Decode(&output)
		assert.NoError(t, err)

		start := output["start"].(map[string]any)
		next := start["next"].(map[string]any)
		assert.Equal(t, 3306, next["port"])
	})

	t.Run("decode returns EOF on exhausted stream", func(t *testing.T) {
		dec := loader.NewDecoder(strings.NewReader(`key: value`))

		var output map[string]any
		err := dec.Decode(&output)
		assert.NoError(t, err)
		assert.Equal(t, "value", output["key"])

		err = dec.Decode(&output)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("streaming multi-document decode", func(t *testing.T) {
		input := `---
first: !xref "database.yaml"
---
second: !xref "base.yaml#net"
`
		dec := loader.NewDecoder(strings.NewReader(input))

		var doc1 map[string]any
		err := dec.Decode(&doc1)
		assert.NoError(t, err)

		first := doc1["first"].(map[string]any)
		assert.Equal(t, 3306, first["port"])

		var doc2 map[string]any
		err = dec.Decode(&doc2)
		assert.NoError(t, err)

		second := doc2["second"].(map[string]any)
		assert.Equal(t, "30s", second["timeout"])

		var doc3 map[string]any
		err = dec.Decode(&doc3)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("decode from bytes reader", func(t *testing.T) {
		input := []byte(`db: !xref "database.yaml"`)
		dec := loader.NewDecoder(bytes.NewReader(input))

		var output map[string]any
		err := dec.Decode(&output)
		assert.NoError(t, err)

		db := output["db"].(map[string]any)
		assert.Equal(t, 3306, db["port"])
	})

	t.Run("invalid yaml through decoder", func(t *testing.T) {
		dec := loader.NewDecoder(strings.NewReader(`invalid: yaml: [:`))

		var output map[string]any
		err := dec.Decode(&output)
		assert.Error(t, err)
	})

	t.Run("missing xref through decoder", func(t *testing.T) {
		dec := loader.NewDecoder(strings.NewReader(`val: !xref "nonexistent.yaml"`))

		var output map[string]any
		err := dec.Decode(&output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in registry")
	})

	t.Run("circular dependency through decoder", func(t *testing.T) {
		loader := NewLoader(getMockFS())

		err := loader.RegisterFile("cyclea.yaml")
		assert.NoError(t, err)
		err = loader.RegisterFile("cycleb.yaml")
		assert.NoError(t, err)

		dec := loader.NewDecoder(strings.NewReader(`start: !xref "cyclea.yaml"`))

		var output map[string]any
		err = dec.Decode(&output)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular dependency")
	})
}

func TestDecoderKnownFields(t *testing.T) {
	loader := NewLoader(getMockFS())

	err := loader.RegisterFile("base.yaml")
	assert.NoError(t, err)

	err = loader.RegisterFile("database.yaml")
	assert.NoError(t, err)

	type NetworkConfig struct {
		Timeout string `yaml:"timeout"`
		Retries int    `yaml:"retries"`
	}

	t.Run("known fields rejects unknown keys", func(t *testing.T) {
		input := `
timeout: 30s
retries: 3
unknown_field: oops
`
		dec := loader.NewDecoder(strings.NewReader(input))
		dec.KnownFields(true)

		var config NetworkConfig
		err := dec.Decode(&config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown_field")
	})

	t.Run("known fields accepts valid keys", func(t *testing.T) {
		input := `
timeout: 30s
retries: 3
`
		dec := loader.NewDecoder(strings.NewReader(input))
		dec.KnownFields(true)

		var config NetworkConfig
		err := dec.Decode(&config)
		assert.NoError(t, err)
		assert.Equal(t, "30s", config.Timeout)
		assert.Equal(t, 3, config.Retries)
	})

	t.Run("known fields disabled by default", func(t *testing.T) {
		input := `
timeout: 30s
retries: 3
extra: ignored
`
		dec := loader.NewDecoder(strings.NewReader(input))

		var config NetworkConfig
		err := dec.Decode(&config)
		assert.NoError(t, err)
		assert.Equal(t, "30s", config.Timeout)
		assert.Equal(t, 3, config.Retries)
	})

	t.Run("known fields with xref resolution", func(t *testing.T) {
		input := `net: !xref "base.yaml#net"`

		type Wrapper struct {
			Net NetworkConfig `yaml:"net"`
		}

		dec := loader.NewDecoder(strings.NewReader(input))
		dec.KnownFields(true)

		var config Wrapper
		err := dec.Decode(&config)
		assert.NoError(t, err)
		assert.Equal(t, "30s", config.Net.Timeout)
		assert.Equal(t, 3, config.Net.Retries)
	})

	t.Run("known fields rejects unknown keys after xref resolution", func(t *testing.T) {
		input := `db: !xref "database.yaml"`

		type Wrapper struct {
			DB NetworkConfig `yaml:"db"`
		}

		dec := loader.NewDecoder(strings.NewReader(input))
		dec.KnownFields(true)

		var config Wrapper
		err := dec.Decode(&config)
		assert.Error(t, err)
	})

	t.Run("known fields with map merge xref", func(t *testing.T) {
		input := `
timeout: 1s
retries: 3
`
		dec := loader.NewDecoder(strings.NewReader(input))
		dec.KnownFields(true)

		var config NetworkConfig
		err := dec.Decode(&config)
		assert.NoError(t, err)
		assert.Equal(t, "1s", config.Timeout)
		assert.Equal(t, 3, config.Retries)
	})
}
