package repositories

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rios0rios0/aisync/internal/domain/entities"
)

// YAMLConfigRepository reads and writes config.yaml files.
type YAMLConfigRepository struct{}

// NewYAMLConfigRepository creates a new YAMLConfigRepository.
func NewYAMLConfigRepository() *YAMLConfigRepository {
	return &YAMLConfigRepository{}
}

// Load reads and parses a config.yaml file.
func (r *YAMLConfigRepository) Load(path string) (*entities.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config entities.Config
	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// Save writes a Config to a YAML file. String values are emitted in
// single-quoted style to follow the project's YAML convention. Booleans,
// integers, floats, and null are kept unquoted so the YAML parser
// preserves their native types on the next Load. Map keys are also left
// unquoted, matching the convention's example output.
func (r *YAMLConfigRepository) Save(path string, config *entities.Config) error {
	var node yaml.Node
	if err := node.Encode(config); err != nil {
		return fmt.Errorf("failed to encode config to yaml node tree: %w", err)
	}
	forceSingleQuotedStringValues(&node)

	data, err := yaml.Marshal(&node)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err = os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Exists checks if a config file exists at the given path.
func (r *YAMLConfigRepository) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// forceSingleQuotedStringValues walks a yaml node tree and forces every
// string scalar VALUE to single-quoted style. Map keys are left at their
// default style (plain) so the output reads naturally and matches the
// YAML convention's example output.
//
// Strings whose contents single-quoted style cannot represent (embedded
// newline or tab) are left unchanged so yaml.v3 picks the correct
// alternative style — typically double-quoted or block.
func forceSingleQuotedStringValues(n *yaml.Node) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range n.Content {
			forceSingleQuotedStringValues(child)
		}
	case yaml.MappingNode:
		// MappingNode.Content is [key0, value0, key1, value1, ...]; descend
		// only into the value half so map keys stay in their default
		// (unquoted) style.
		for i := 1; i < len(n.Content); i += 2 {
			forceSingleQuotedStringValues(n.Content[i])
		}
	case yaml.ScalarNode:
		if n.Tag == "!!str" && canUseSingleQuoted(n.Value) {
			n.Style = yaml.SingleQuotedStyle
		}
	case yaml.AliasNode:
		// Aliases reference other nodes; the referent is rewritten in its
		// own visit.
	}
}

// canUseSingleQuoted reports whether s can be emitted as a YAML
// single-quoted scalar. Single-quoted style cannot contain raw newlines
// or tabs — those must use double-quoted or block style.
func canUseSingleQuoted(s string) bool {
	return !strings.ContainsAny(s, "\n\t")
}
