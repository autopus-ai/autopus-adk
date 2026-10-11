package cli

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

const utf8ByteOrderMark = "\xef\xbb\xbf"

// spliceQualityBlock returns data with its top-level quality block replaced by
// the block that replaceAutopusConfigSection encodes for quality, or with that
// block appended when data has none. Every byte outside the block is kept;
// the block itself is re-encoded, so its comments and layout are not.
func spliceQualityBlock(data []byte, source *qualityWriteSource, quality config.QualityConf) ([]byte, error) {
	root := source.root
	if root.Style&yaml.FlowStyle != 0 || len(root.Content) == 0 || root.Content[0].Column != 1 {
		return nil, fmt.Errorf("refuse to write quality: autopus.yaml must be a block mapping whose keys " +
			"start at column 1; edit autopus.yaml by hand")
	}
	encoded, err := replaceAutopusConfigSection(data, "quality", quality)
	if err != nil {
		return nil, err
	}
	block, err := encodedQualityBlock(encoded)
	if err != nil {
		return nil, err
	}
	if newline := detectConfigNewline(data); newline != "\n" {
		block = strings.ReplaceAll(block, "\n", newline)
	}

	lines := strings.SplitAfter(string(data), "\n")
	start, end := len(lines), len(lines)
	if source.keyIndex >= 0 {
		start, end = qualityBlockRange(lines, root, source.keyIndex)
		// The byte order mark of a file that opens with quality is not part
		// of the block.
		if start == 0 && strings.HasPrefix(lines[0], utf8ByteOrderMark) {
			block = utf8ByteOrderMark + block
		}
	} else if marker := yamlDocumentMarkerLine(lines, root.Line); marker >= 0 {
		start, end = marker, marker
	} else if last := lines[len(lines)-1]; last != "" {
		// A file without a final newline gets one before the appended block.
		lines[len(lines)-1] = last + detectConfigNewline(data)
	}
	var out strings.Builder
	out.WriteString(strings.Join(lines[:start], ""))
	out.WriteString(block)
	out.WriteString(strings.Join(lines[end:], ""))
	return []byte(out.String()), nil
}

// encodedQualityBlock cuts the quality block out of a document that yaml.v3
// encoded, by the same boundary rule that locates the block being replaced.
func encodedQualityBlock(encoded []byte) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(encoded, &doc); err != nil || len(doc.Content) == 0 {
		return "", errors.New("autopus_config_marshal_failed")
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "quality" {
			continue
		}
		lines := strings.SplitAfter(string(encoded), "\n")
		start, end := qualityBlockRange(lines, root, i)
		block := strings.Join(lines[start:end], "")
		if !strings.HasSuffix(block, "\n") {
			block += "\n"
		}
		return block, nil
	}
	return "", errors.New("autopus_config_marshal_failed")
}

// qualityBlockRange returns the line range [start, end) of the root mapping
// entry at keyIndex (SPEC-MODELUX-001 REQ-MUX-08, R05). The range runs from
// the key's line to the line before the next root key, or to the end of the
// document when the entry is the last one. Blank lines and column-one
// comments at the end of that range belong to whatever follows, so they stay
// outside; a column-one comment followed by an indented child stays inside.
func qualityBlockRange(lines []string, root *yaml.Node, keyIndex int) (int, int) {
	start := root.Content[keyIndex].Line - 1
	end := len(lines)
	if keyIndex+2 < len(root.Content) {
		end = root.Content[keyIndex+2].Line - 1
	} else if marker := yamlDocumentMarkerLine(lines, start+1); marker >= 0 {
		end = marker
	}
	for end > start+1 && isQualityBlockTrailer(lines[end-1]) {
		end--
	}
	return start, end
}

// yamlDocumentMarkerLine returns the index of the first line at or after from
// that starts or ends a YAML document, or -1.
func yamlDocumentMarkerLine(lines []string, from int) int {
	for i := max(from, 0); i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r\n")
		for _, marker := range []string{"---", "..."} {
			if line == marker || strings.HasPrefix(line, marker+" ") || strings.HasPrefix(line, marker+"\t") {
				return i
			}
		}
	}
	return -1
}

func isQualityBlockTrailer(line string) bool {
	line = strings.TrimRight(line, "\r\n")
	return strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#")
}
