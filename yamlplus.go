package yamlplus

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

type Loader struct {
	Filesystem fs.FS

	anchorRegistry map[string]*yaml.Node
}

func NewLoader(f fs.FS) *Loader {
	return &Loader{
		Filesystem:     f,
		anchorRegistry: make(map[string]*yaml.Node),
	}
}

func (l *Loader) RegisterFile(path string) error {
	f, err := l.Filesystem.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var root yaml.Node
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&root); err != nil {
		return err
	}

	if len(root.Content) > 0 {
		// set up an anchor directly to the document so !xref "somefile.yaml"
		// works
		l.anchorRegistry[path] = root.Content[0]

		for _, doc := range root.Content {
			l.scanAnchors(path, doc)
		}
	}

	return nil
}

func (l *Loader) RegisterDirectory(dir string) error {
	entries, err := fs.ReadDir(l.Filesystem, dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && hasYAMLSuffix(entry.Name()) {
			fullPath := path.Join(dir, entry.Name())

			if err := l.RegisterFile(fullPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func (l *Loader) RegisterRecursively(dir string) error {
	return fs.WalkDir(l.Filesystem, dir, func(currentPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.IsDir() && hasYAMLSuffix(entry.Name()) {
			if err := l.RegisterFile(currentPath); err != nil {
				return err
			}
		}

		return nil
	})
}

func (l *Loader) Unmarshal(data []byte, out any) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}

	stack := make(map[string]bool)

	if err := l.replaceXrefs(&root, stack); err != nil {
		return err
	}

	return root.Decode(out)
}

// Go through a YAML node and its children and replace occurences of nodes that
// are tagged with `!xref` with the node they refer to
func (l *Loader) replaceXrefs(node *yaml.Node, stack map[string]bool) error {
	if node == nil {
		return nil
	}

	if node.Tag == "!xref" {
		// we can directly return in this case since resolveDirectXRef
		// recurses
		return l.resolveDirectXRef(node, stack)
	}

	if node.Kind == yaml.MappingNode {
		if err := l.resolveMapMergeXRef(node, stack); err != nil {
			return err
		}
	}

	for _, child := range node.Content {
		if err := l.replaceXrefs(child, stack); err != nil {
			return err
		}
	}

	return nil
}

// Get a (previously registered) anchor by reference.
func (l *Loader) getAnchor(ref string) (*yaml.Node, error) {
	if target, ok := l.anchorRegistry[ref]; ok {
		return target, nil
	}

	return nil, fmt.Errorf("xref %q not found in registry %v", ref, l.anchorRegistry)
}

// Scan a node and its children for any anchors and store them in the
// anchor registry. The path is included for namespacing.
func (l *Loader) scanAnchors(path string, node *yaml.Node) {
	if node == nil {
		return
	}

	if node.Anchor != "" {
		l.anchorRegistry[fmt.Sprintf("%s#%s", path, node.Anchor)] = node
	}

	for _, child := range node.Content {
		l.scanAnchors(path, child)
	}
}

func (l *Loader) applyMerge(src, dst *yaml.Node) int {
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		return 0
	}

	existingKeys := make(map[string]bool)
	for i := 0; i < len(dst.Content); i += 2 {
		existingKeys[dst.Content[i].Value] = true
	}

	newKeyVals := make([]*yaml.Node, 0)
	for i := 0; i < len(src.Content); i += 2 {
		key := src.Content[i]
		val := src.Content[i+1]
		if !existingKeys[key.Value] {
			newKeyVals = append(newKeyVals, key, val)
		}
	}

	dst.Content = append(newKeyVals, dst.Content...)

	return len(newKeyVals)
}

func (l *Loader) resolveDirectXRef(node *yaml.Node, stack map[string]bool) error {
	if stack[node.Value] {
		return fmt.Errorf("circular dependency detected: %q", node.Value)
	}

	stack[node.Value] = true

	resolved, err := l.getAnchor(node.Value)
	if err != nil {
		return err
	}

	// for a direct reference to a document we want its first child (mapping
	// or sequence) instead of the DocumentNode itself. note that when files
	// are registered we verify that they are non-empty so one can never
	// reference an empty document
	if resolved.Kind == yaml.DocumentNode {
		resolved = resolved.Content[0]
	}

	clone := cloneNode(resolved)

	node.Kind = clone.Kind
	node.Style = clone.Style
	node.Tag = clone.Tag
	node.Value = clone.Value
	node.Content = clone.Content

	delete(stack, node.Value)

	return l.replaceXrefs(node, stack)
}

func (l *Loader) resolveMapMergeXRef(node *yaml.Node, stack map[string]bool) error {
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		val := node.Content[i+1]

		if key.Value == "<<" {
			toMerge, err := l.extractMapMergeTargets(val)
			if err != nil {
				return err
			}

			if len(toMerge) > 0 {
				node.Content = append(node.Content[:i], node.Content[i+2:]...)
				newKeys := 0

				// apply in sequence order, the yaml spec says that earlier items in the sequence
				// take precedence
				for _, src := range toMerge {
					var resolved *yaml.Node

					if src.Tag == "!xref" {
						if stack[src.Value] {
							return fmt.Errorf("circular dependency detected: %q", src.Value)
						}

						stack[src.Value] = true

						var err error

						resolved, err = l.getAnchor(src.Value)
						if err != nil {
							return err
						}

						if resolved.Kind == yaml.DocumentNode {
							resolved = resolved.Content[0]
						}

						if err := l.replaceXrefs(resolved, stack); err != nil {
							return err
						}

						delete(stack, src.Value)
					} else {
						resolved = src
						if err := l.replaceXrefs(resolved, stack); err != nil {
							return err
						}
					}

					newKeys += l.applyMerge(resolved, node)
				}

				i = i + newKeys - 2
			}
		}
	}

	return nil
}

func (l *Loader) extractMapMergeTargets(val *yaml.Node) ([]*yaml.Node, error) {
	var targets []*yaml.Node

	if val.Tag == "!xref" || val.Kind == yaml.MappingNode {
		targets = append(targets, val)
		return targets, nil
	}

	if val.Kind == yaml.SequenceNode {
		for _, item := range val.Content {
			if item.Tag == "!xref" || item.Kind == yaml.MappingNode {
				targets = append(targets, item)
			} else if item.Kind == yaml.AliasNode && item.Alias != nil {
				targets = append(targets, item.Alias)
			}
		}
	}
	return targets, nil
}

func cloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	clone := *n // copy value types (Kind, Style, Tag, Value, etc.)

	if n.Content != nil {
		clone.Content = make([]*yaml.Node, len(n.Content))
		for i, child := range n.Content {
			clone.Content[i] = cloneNode(child)
		}
	}
	return &clone
}
func hasYAMLSuffix(name string) bool {
	suffix := strings.ToLower(path.Ext(name))
	return suffix == ".yaml" || suffix == ".yml"
}
