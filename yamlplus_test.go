package yamlplus

import (
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

	return fstest.MapFS{
		"base.yaml":     {Data: baseYAML},
		"database.yaml": {Data: databaseYAML},
		"multidoc.yaml": {Data: multidocYAML},
		"nested.yaml":   {Data: nestedYAML},
		"cyclea.yaml":   {Data: cycleaYAML},
		"cycleb.yaml":   {Data: cyclebYAML},
		"empty.yaml":    {Data: emptyYAML},
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
