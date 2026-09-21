package setup

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry is one key we set on a system. Value is a string, []string, int or
// bool.
type Entry struct {
	Key   string
	Value any
}

// Document is a configuration file kept as a node tree. Editing nodes rather
// than marshalling structs is what preserves comments, key order and the
// keys other tools put in the same file.
type Document struct {
	root *yaml.Node
}

func LoadDocument(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Document{root: newMappingDoc()}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if root.Kind == 0 {
		return &Document{root: newMappingDoc()}, nil
	}
	// yaml.Unmarshal silently keeps only the first of several `---`
	// separated documents; editing that copy and writing it back would
	// discard the rest of the file without ever saying so. A second Decode
	// on the same bytes tells us whether there was more to lose.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var first yaml.Node
	_ = dec.Decode(&first)
	var second yaml.Node
	if dec.Decode(&second) == nil {
		return nil, fmt.Errorf("%s has more than one YAML document (a %q separator) — only the first would be read and the rest lost; split the file or edit it by hand", path, "---")
	}
	return &Document{root: &root}, nil
}

func newMappingDoc() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{{
			Kind: yaml.MappingNode, Tag: "!!map",
		}},
	}
}

func (d *Document) UpsertSystem(name string, entries []Entry) (bool, error) {
	key, err := d.systemsKey()
	if err != nil {
		return false, err
	}
	return d.upsert(key, name, entries)
}

func (d *Document) UpsertSSHSystem(name string, entries []Entry) (bool, error) {
	return d.upsert("ssh_systems", name, entries)
}

// EnsureDefaultTransport adds transport: {stdio: true} when the file has no
// transport key at all, so it can start the server unassisted. An existing
// block, of any shape, is left alone — this only fills a gap.
func (d *Document) EnsureDefaultTransport() (bool, error) {
	m, err := d.mapping()
	if err != nil {
		return false, err
	}
	if mapValue(m, "transport") != nil {
		return false, nil
	}
	block := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setMapValue(block, "stdio", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	setMapValue(m, "transport", block)
	return true, nil
}

// systemsKey keeps the spelling the file already uses and falls back to the
// one the other tools write.
func (d *Document) systemsKey() (string, error) {
	m, err := d.mapping()
	if err != nil {
		return "", err
	}
	if mapValue(m, "systems") != nil {
		return "systems", nil
	}
	return "servers", nil
}

// mapping returns the document's top-level node, erroring when it is not a
// mapping — a file whose root is, say, a bare scalar has nowhere to hang a
// `servers:` key, and upsert()'s node-append onto a non-mapping node is
// silently discarded by the encoder rather than raising anything.
func (d *Document) mapping() (*yaml.Node, error) {
	if len(d.root.Content) == 0 {
		return nil, fmt.Errorf("the configuration is empty — edit it by hand, or point --file at a different one")
	}
	m := d.root.Content[0]
	if m.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("the configuration's top level is %s, not a mapping — edit the file by hand, or point --file at a different one", describeKind(m.Kind))
	}
	return m, nil
}

func describeKind(k yaml.Kind) string {
	switch k {
	case yaml.ScalarNode:
		return "a single value"
	case yaml.SequenceNode:
		return "a list"
	case yaml.AliasNode:
		return "an alias"
	default:
		return "not a mapping"
	}
}

func (d *Document) upsert(sectionKey, name string, entries []Entry) (bool, error) {
	m, err := d.mapping()
	if err != nil {
		return false, err
	}
	section := mapValue(m, sectionKey)
	if section == nil {
		section = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		setMapValue(m, sectionKey, section)
	}
	if section.Kind != yaml.SequenceNode {
		return false, fmt.Errorf("%s in the configuration is not a list", sectionKey)
	}

	entry := findByName(section, name)
	changed := false
	if entry == nil {
		entry = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMapValue(entry, "name", scalar(name))
		section.Content = append(section.Content, entry)
		changed = true
	}
	for _, e := range entries {
		node, err := valueNode(e.Value)
		if err != nil {
			return false, fmt.Errorf("%s: %w", e.Key, err)
		}
		if existing := mapValue(entry, e.Key); existing != nil && sameValue(existing, node) {
			continue
		}
		setMapValue(entry, e.Key, node)
		changed = true
	}
	return changed, nil
}

func findByName(seq *yaml.Node, name string) *yaml.Node {
	for _, item := range seq.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		if n := mapValue(item, "name"); n != nil && strings.EqualFold(n.Value, name) {
			return item
		}
	}
	return nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func setMapValue(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			// Keep the key node, and with it any comment attached to it.
			// An inline trailing comment sits on the value node, not the
			// key node, so it has to be carried onto the replacement too.
			carryComments(m.Content[i+1], value)
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, scalar(key), value)
}

// carryComments copies a replaced node's comments onto its replacement,
// unless the replacement already carries its own.
func carryComments(old, replacement *yaml.Node) {
	if replacement.HeadComment == "" {
		replacement.HeadComment = old.HeadComment
	}
	if replacement.LineComment == "" {
		replacement.LineComment = old.LineComment
	}
	if replacement.FootComment == "" {
		replacement.FootComment = old.FootComment
	}
}

func scalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func valueNode(v any) (*yaml.Node, error) {
	switch t := v.(type) {
	case string:
		return scalar(t), nil
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(t)}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(t)}, nil
	case []string:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, s := range t {
			seq.Content = append(seq.Content, scalar(s))
		}
		return seq, nil
	case map[string]string:
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			m.Content = append(m.Content, scalar(k), scalar(t[k]))
		}
		return m, nil
	default:
		return nil, fmt.Errorf("unsupported value type %T", v)
	}
}

// sameValue reports whether two nodes hold the same value. Comments are not
// part of the value: a freshly built replacement never carries one of its
// own, so comparing them would always call a re-set of an unchanged value
// "different" once the existing node has a comment on it — which is exactly
// the write setMapValue must recognize as a no-op and leave alone.
func sameValue(a, b *yaml.Node) bool {
	if a.Kind != b.Kind || a.Tag != b.Tag || a.Value != b.Value {
		return false
	}
	if len(a.Content) != len(b.Content) {
		return false
	}
	for i := range a.Content {
		if !sameValue(a.Content[i], b.Content[i]) {
			return false
		}
	}
	return true
}

func render(n *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (d *Document) Bytes() ([]byte, error) {
	return render(d.root)
}

// Save writes a backup beside the file the first time it replaces it.
func (d *Document) Save(path string) error {
	out, err := d.Bytes()
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", existing, 0o600); err != nil {
			return fmt.Errorf("writing the backup: %w", err)
		}
	}
	return os.WriteFile(path, out, 0o600)
}
