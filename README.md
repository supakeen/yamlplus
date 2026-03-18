# `yamlplus`

This is a small Go library that processes the Abstract Syntax Tree (AST) offered by `go.yaml.in/yaml/v3` to extend it with tags.

## Usage

```go
package main

import (
    "os"

    "github.com/supakeen/yamlplus"
)

func main() {
    loader := yamlplus.NewLoader(os.DirFS("somepath"))

    _ = loader.RegisterFile("one.yaml")
    _ = loader.RegisterFile("two.yaml")

    _ = loader.RegisterDirectory("dir")

    _ = loader.RegisterRecursively("dir")

    var output map[string]any

    _ = loader.Unmarshal([]byte(`one: !xref "one.yaml#anchor")`, output)
}
```

## Tags

### `!xref`

The `!xref` tag allows for cross-referencing anchors in other YAML files.

```yaml
config:
  port: &port 3306
```

```yaml
other_config:
  port: !xref config.yaml#port
```

The syntax is `!xref` followed by the filename of the file the anchor appears in, a `#` and then the name of the anchor. You can reference an entire file by omitting the `#anchor` part. When referencing an entire file but that file contains multiple documents the first one will be used implicitly.

`!xref` is also available for map merges:

```yaml
config:
  <<: !xref base.yaml#config
  port: 3306
```
