package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// errQualityWriteRefused is the refusal of saveQualityBlock, and of the
// composition step's line write, when the quality block of autopus.yaml
// cannot serve as a write source (SPEC-MODELUX-001 REQ-MUX-08). The file
// stays as it is.
var errQualityWriteRefused = errors.New(
	"quality 블록에 환경 변수 값이나 YAML 참조(alias·merge)가 섞여 있어 자동으로 쓰지 않습니다. autopus.yaml을 직접 고치세요.")

// qualityWriteSource is the quality section of autopus.yaml bytes that a
// writer may build on. config.Load expands ${NAME} across the whole text
// before it decodes, so the loaded quality can hold an operator's environment
// value (a token) that must never be written back (F-001). The source is
// instead the raw quality node decoded without expansion, and it is accepted
// only when that decode equals the loaded quality and the subtree holds no
// alias or merge key: re-encoding would turn a reference into a copy, and a
// reference to an anchor outside quality can carry an expanded value into
// the block without a ${ inside it.
type qualityWriteSource struct {
	doc      yaml.Node
	root     *yaml.Node            // root mapping
	keyIndex int                   // index of the quality key in root.Content, or -1
	quality  *yaml.Node            // quality value node, or nil
	loaded   *config.HarnessConfig // what config.Load reads from the same bytes
}

// readQualityWriteSource runs the write precheck on data, the autopus.yaml
// bytes after pruneRetiredConfig. It returns errQualityWriteRefused when the
// raw quality differs from the loaded one or holds a YAML reference.
func readQualityWriteSource(data []byte) (*qualityWriteSource, error) {
	source := &qualityWriteSource{keyIndex: -1}
	if err := yaml.Unmarshal(data, &source.doc); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if len(source.doc.Content) == 0 || source.doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config root must be a YAML mapping")
	}
	loaded, err := config.ParseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	source.loaded = loaded
	source.root = source.doc.Content[0]
	for i := 0; i+1 < len(source.root.Content); i += 2 {
		if key := source.root.Content[i]; key.Kind == yaml.ScalarNode && key.Value == "quality" {
			source.keyIndex, source.quality = i, source.root.Content[i+1]
			break
		}
	}
	if yamlSubtreeHasReference(source.quality) {
		return nil, errQualityWriteRefused
	}
	raw, err := source.rawQuality()
	if err != nil || !reflect.DeepEqual(raw, loaded.Quality) {
		return nil, errQualityWriteRefused
	}
	return source, nil
}

// precheckQualityWriteFile runs the write precheck on dir's autopus.yaml the
// way saveQualityBlock reads it, for a writer that runs none of its own. A
// missing file passes: the writer creates it from code defaults.
func precheckQualityWriteFile(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	if data, _, err = pruneRetiredConfig(data); err != nil {
		return err
	}
	_, err = readQualityWriteSource(data)
	return err
}

// rawQuality decodes the quality node without environment expansion. Every
// call returns maps of its own, so a caller may edit the result.
func (s *qualityWriteSource) rawQuality() (config.QualityConf, error) {
	var quality config.QualityConf
	if s.quality == nil {
		return quality, nil
	}
	err := s.quality.Decode(&quality)
	return quality, err
}

// yamlSubtreeHasReference reports whether node or any node below it is an
// alias or a merge key. An alias is a leaf of the parsed tree, so the walk
// never follows one and terminates.
func yamlSubtreeHasReference(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.AliasNode {
		return true
	}
	for i, child := range node.Content {
		if node.Kind == yaml.MappingNode && i%2 == 0 && child.Kind == yaml.ScalarNode && child.ShortTag() == "!!merge" {
			return true
		}
		if yamlSubtreeHasReference(child) {
			return true
		}
	}
	return false
}
