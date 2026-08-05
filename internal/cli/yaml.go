package cli

import (
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/rwx-cloud/rwx/internal/errors"
)

type YAMLDoc struct {
	astFile  *ast.File
	original string
	latest   *string
}

func ParseYAMLDoc(content string) (*YAMLDoc, error) {
	astFile, err := parser.ParseBytes([]byte(content), parser.ParseComments)
	if err != nil {
		return nil, err
	}
	latest := astFile.String()

	return &YAMLDoc{astFile: astFile, original: latest, latest: &latest}, nil
}

func ParseYAMLFile(path string) (*YAMLDoc, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return ParseYAMLDoc(string(content))
}

func (doc *YAMLDoc) Bytes() []byte {
	return []byte(doc.String())
}

func (doc *YAMLDoc) String() string {
	if doc.latest == nil {
		s := doc.astFile.String()
		doc.latest = &s
	}
	return *doc.latest
}

func (doc *YAMLDoc) HasChanges() bool {
	return doc.original != doc.String()
}

func (doc *YAMLDoc) HasBase() bool {
	return doc.hasPath("$.base")
}

func (doc *YAMLDoc) HasBaseOs() bool {
	return doc.hasPath("$.base.os")
}

func (doc *YAMLDoc) HasBaseTag() bool {
	return doc.hasPath("$.base.tag")
}

func (doc *YAMLDoc) HasTasks() bool {
	return doc.hasPath("$.tasks")
}

// packagesDirCallRe matches calls into the local packages directory, eg.
// `call: ${{ run.dir }}/packages/my-package`
var packagesDirCallRe = regexp.MustCompile(`^\$\{\{\s*run\.dir\s*\}\}/packages/`)

func (doc *YAMLDoc) AllTasksAreEmbeddedRuns() bool {
	err := doc.ForEachNode("$.tasks[*]", func(node ast.Node) error {
		mapNode, ok := node.(*ast.MappingNode)
		if !ok {
			return fmt.Errorf("expected mapping node, got %T", node)
		}

		callValue := ""
		hasPackageMarker := false
		for _, entry := range mapNode.Values {
			switch entry.Key.String() {
			case "call":
				callValue = entry.Value.String()
			case "with", "use":
				// `with` and `use` are only valid on package calls, so their
				// presence means this is a local package call, not an embedded run.
				hasPackageMarker = true
			}
		}

		isEmbeddedRun := strings.HasPrefix(callValue, "${{") &&
			!hasPackageMarker &&
			!packagesDirCallRe.MatchString(callValue)

		if !isEmbeddedRun {
			return errors.New("no embedded run found")
		}

		return nil
	})

	return err == nil
}

func (doc *YAMLDoc) IsRunDefinition() bool {
	if len(doc.astFile.Docs) != 1 {
		// Multi-document files are not supported
		return false
	}

	yamlDoc := doc.astFile.Docs[0]
	if yamlDoc.Body == nil {
		// Empty document
		return false
	}

	return yamlDoc.Body.Type() == ast.MappingType && doc.HasTasks()
}

func (doc *YAMLDoc) IsListOfTasks() bool {
	if len(doc.astFile.Docs) != 1 {
		// Multi-document files are not supported
		return false
	}

	yamlDoc := doc.astFile.Docs[0]
	if yamlDoc.Body == nil {
		// Empty document
		return false
	}

	return yamlDoc.Body.Type() == ast.SequenceType
}

func (doc *YAMLDoc) ReadStringAtPath(yamlPath string) (string, error) {
	node, err := doc.getNodeAtPath(yamlPath)
	if err != nil {
		return "", err
	}

	return node.String(), nil
}

func (doc *YAMLDoc) TryReadStringAtPath(yamlPath string) string {
	str, err := doc.ReadStringAtPath(yamlPath)
	if err != nil {
		return ""
	}
	return str
}

func (doc *YAMLDoc) InsertBefore(beforeYamlPath string, value any) error {
	if strings.Count(beforeYamlPath, ".") != 1 {
		return errors.New("must provide a root yaml field in the form of \"$.fieldname\"")
	}

	p, err := yaml.PathString(beforeYamlPath)
	if err != nil {
		panic(err)
	}

	// We can't use doc.astFile because it may have already been modified and
	// we need the original index for the relative yaml node.
	reparsedFile, err := parser.ParseBytes([]byte(doc.astFile.String()), parser.ParseComments)
	if err != nil {
		return err
	}

	relativeNode, err := p.FilterFile(reparsedFile)
	if err != nil {
		return err
	}

	// token: value for the given beforeYamlPath
	// token.Prev: the separator token, eg. ":"
	// token.Prev.Prev: key for the given beforeYamlPath
	token := relativeNode.GetToken()
	if token.Prev == nil {
		return errors.New("unexpected token structure: token.Prev is nil")
	}
	if token.Prev.Prev == nil {
		return errors.New("unexpected token structure: token.Prev.Prev is nil")
	}

	// Find the start of the line containing the anchor key.
	keyToken := token.Prev.Prev
	content := []byte(doc.astFile.String())

	// Use line numbers instead of Position.Offset, which is unreliable
	// when the document contains comments.
	lineStarts := lineStartOffsets(content)
	idx := lineStarts[keyToken.Position.Line-1]

	// Look backwards to find any preceding comment lines and insert before them.
	// This preserves comment blocks by inserting before the entire block.
	for idx > 0 {
		// Find the start of the previous line
		lineStart := idx - 1
		for lineStart > 0 && content[lineStart-1] != '\n' {
			lineStart--
		}

		// Check if the previous line is a comment
		lineContent := content[lineStart : idx-1]
		trimmed := strings.TrimSpace(string(lineContent))
		if strings.HasPrefix(trimmed, "#") {
			idx = lineStart
		} else {
			break
		}
	}

	node, err := yaml.NewEncoder(nil).EncodeToNode(value)
	if err != nil {
		return err
	}

	toInsert := fmt.Appendf([]byte(node.String()), "\n\n")
	result := slices.Insert([]byte(doc.astFile.String()), idx, toInsert...)

	err = doc.reparseAst(string(result))
	if err != nil {
		return err
	}

	return nil
}

func (doc *YAMLDoc) MergeAtPath(yamlPath string, value any) error {
	p, err := yaml.PathString(yamlPath)
	if err != nil {
		panic(err)
	}

	node, err := yaml.NewEncoder(nil).EncodeToNode(value)
	if err != nil {
		return err
	}

	err = p.MergeFromNode(doc.astFile, node)
	if err != nil {
		return err
	}

	doc.modified()
	return nil
}

func (doc *YAMLDoc) ReplaceAtPath(yamlPath string, replacement any) error {
	p, err := yaml.PathString(yamlPath)
	if err != nil {
		panic(err)
	}

	// Ensure the path exists
	if _, err := p.FilterFile(doc.astFile); err != nil {
		return err
	}

	node, err := yaml.NewEncoder(nil).EncodeToNode(replacement)
	if err != nil {
		return err
	}

	err = p.ReplaceWithNode(doc.astFile, node)
	if err != nil {
		return err
	}

	doc.modified()
	return nil
}

// ReplaceRootField replaces a root-level field with a new value, preserving
// proper YAML formatting. Use this for replacing entire sections like "base:".
func (doc *YAMLDoc) ReplaceRootField(fieldName string, value any) error {
	yamlPath := "$." + fieldName
	p, err := yaml.PathString(yamlPath)
	if err != nil {
		return err
	}

	// We need to reparse to get accurate token positions
	reparsedFile, err := parser.ParseBytes([]byte(doc.astFile.String()), parser.ParseComments)
	if err != nil {
		return err
	}

	fieldNode, err := p.FilterFile(reparsedFile)
	if err != nil {
		return err
	}

	// Get the position of the field value
	token := fieldNode.GetToken()
	if token.Prev == nil || token.Prev.Prev == nil {
		return errors.New("unexpected token structure")
	}

	// Find the start of the line containing the key
	keyToken := token.Prev.Prev
	content := []byte(doc.astFile.String())

	// Use line numbers instead of Position.Offset, which is unreliable
	// when the document contains comments.
	lineStarts := lineStartOffsets(content)
	startIdx := lineStarts[keyToken.Position.Line-1]

	// Find the end of this field section (next root-level key or EOF)
	endIdx := len(content)

	// Get the root mapping node to find all root-level keys
	if len(reparsedFile.Docs) > 0 && reparsedFile.Docs[0].Body != nil {
		if mapNode, ok := reparsedFile.Docs[0].Body.(*ast.MappingNode); ok {
			for _, entry := range mapNode.Values {
				entryLine := entry.Key.GetToken().Position.Line
				// Find the next key after our field
				if entryLine > keyToken.Position.Line {
					nextStart := lineStarts[entryLine-1]
					// Also include any blank lines before the next field
					for nextStart > startIdx && nextStart > 0 && content[nextStart-1] == '\n' {
						checkIdx := nextStart - 1
						for checkIdx > 0 && content[checkIdx-1] != '\n' {
							checkIdx--
						}
						line := strings.TrimSpace(string(content[checkIdx : nextStart-1]))
						if line == "" || strings.HasPrefix(line, "#") {
							nextStart = checkIdx
						} else {
							break
						}
					}
					endIdx = nextStart
					break
				}
			}
		}
	}

	// Encode the new value
	newNode, err := yaml.NewEncoder(nil).EncodeToNode(map[string]any{fieldName: value})
	if err != nil {
		return err
	}

	// Build the replacement
	newContent := newNode.String()
	if endIdx < len(content) {
		newContent += "\n"
	}

	// Replace the section
	result := string(content[:startIdx]) + newContent + string(content[endIdx:])

	return doc.reparseAst(result)
}

func (doc *YAMLDoc) SetAtPath(yamlPath string, value any) error {
	pathParts := strings.Split(yamlPath, ".")
	field := pathParts[len(pathParts)-1]

	parent := strings.Join(pathParts[0:len(pathParts)-1], ".")
	path, err := yaml.PathString(parent)
	if err != nil {
		panic(err)
	}

	node, err := yaml.NewEncoder(nil).EncodeToNode(map[string]any{
		field: value,
	})
	if err != nil {
		return err
	}

	err = path.MergeFromNode(doc.astFile, node)
	if err != nil {
		return err
	}

	doc.modified()
	return nil
}

func (doc *YAMLDoc) ForEachNode(yamlPath string, f func(node ast.Node) error) error {
	node, err := doc.getNodeAtPath(yamlPath)
	if err != nil {
		// The yamlPath isn't compatible with the underlying YAML doc, for instance
		// an sequence of strings where we expect a sequence of maps.
		if errors.Is(err, yaml.ErrInvalidQuery) || errors.Is(err, yaml.ErrNotFoundNode) {
			return nil
		}
		return err
	}

	seqNode, ok := node.(*ast.SequenceNode)
	if !ok {
		return fmt.Errorf("expected sequence node, got %T", node)
	}

	for _, valueNode := range seqNode.Values {
		if valueNode == nil {
			continue
		}
		if err := f(valueNode); err != nil {
			return err
		}
	}
	return nil
}

func (doc *YAMLDoc) WriteFile(path string) error {
	// Inherit permissions from the existing file if it exists
	mode := fs.FileMode(0644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode()
	}

	return os.WriteFile(path, doc.Bytes(), mode)
}

func (doc *YAMLDoc) getNodeAtPath(yamlPath string) (ast.Node, error) {
	p, err := yaml.PathString(yamlPath)
	if err != nil {
		panic(err)
	}

	return p.FilterFile(doc.astFile)
}

func (doc *YAMLDoc) hasPath(yamlPath string) bool {
	_, err := doc.getNodeAtPath(yamlPath)
	return err == nil
}

func (doc *YAMLDoc) modified() {
	doc.latest = nil
}

// lineStartOffsets returns a slice where index i holds the byte offset of the
// start of line i+1 (1-based line numbers). This is used instead of
// token.Position.Offset which is unreliable in the presence of YAML comments.
func lineStartOffsets(content []byte) []int {
	starts := []int{0} // line 1 starts at byte 0
	for i, b := range content {
		if b == '\n' && i+1 < len(content) {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func (doc *YAMLDoc) reparseAst(contents string) error {
	astFile, err := parser.ParseBytes([]byte(contents), parser.ParseComments)
	if err != nil {
		return err
	}

	doc.astFile = astFile
	doc.latest = nil
	return nil
}
