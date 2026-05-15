package project

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var defaultIgnore = []string{
	".git",
	"node_modules",
	"vendor",
	"bin",
	".DS_Store",
	"__pycache__",
	".venv",
	"target",
}

// DetectRoot finds the project root by walking up from cwd.
// Returns (root, true) if a project marker was found, or ("", false) otherwise.
func DetectRoot() (string, bool) {
	dir, _ := os.Getwd()
	markers := []string{".git", "go.mod", "package.json", "Cargo.toml", "pyproject.toml"}

	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", false
}

func Tree(root string, maxDepth int) string {
	ignoreSet := make(map[string]bool)
	for _, p := range defaultIgnore {
		ignoreSet[p] = true
	}

	gitignore := loadGitignore(root)
	for _, p := range gitignore {
		ignoreSet[p] = true
	}

	var sb strings.Builder
	sb.WriteString(".\n")
	buildTree(&sb, root, "", maxDepth, 0, ignoreSet)
	return sb.String()
}

func buildTree(sb *strings.Builder, dir, prefix string, maxDepth, depth int, ignore map[string]bool) {
	if depth >= maxDepth {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var visible []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if ignore[name] {
			continue
		}
		if strings.HasPrefix(name, ".") && name != "." {
			continue
		}
		visible = append(visible, e)
	}

	for i, e := range visible {
		isLast := i == len(visible)-1
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		name := e.Name()
		if e.IsDir() {
			sb.WriteString(fmt.Sprintf("%s%s%s/\n", prefix, connector, name))
			buildTree(sb, filepath.Join(dir, name), childPrefix, maxDepth, depth+1, ignore)
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s\n", prefix, connector, name))
		}
	}
}

func loadGitignore(root string) []string {
	path := filepath.Join(root, ".gitignore")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSuffix(line, "/")
		patterns = append(patterns, line)
	}
	return patterns
}

const maxFileSize = 8 * 1024

func ReadFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot stat %s: %w", path, err)
	}

	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)
	if len(content) > maxFileSize {
		content = content[:maxFileSize] + "\n...(truncated at 8KB)..."
	}

	return content, nil
}
