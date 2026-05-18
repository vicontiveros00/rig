package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTree(t *testing.T) {
	dir := t.TempDir()

	// Create structure
	os.MkdirAll(filepath.Join(dir, "src", "utils"), 0o755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "src", "app.go"), []byte("package src"), 0o644)
	os.WriteFile(filepath.Join(dir, "src", "utils", "helper.go"), []byte("package utils"), 0o644)

	tree := Tree(dir, 4)

	if !strings.Contains(tree, "main.go") {
		t.Error("expected tree to contain main.go")
	}
	if !strings.Contains(tree, "src/") {
		t.Error("expected tree to contain src/")
	}
	if !strings.Contains(tree, "helper.go") {
		t.Error("expected tree to contain helper.go")
	}
}

func TestTreeRespectsMaxDepth(t *testing.T) {
	dir := t.TempDir()

	// Create deep structure
	deep := filepath.Join(dir, "a", "b", "c", "d", "e")
	os.MkdirAll(deep, 0o755)
	os.WriteFile(filepath.Join(deep, "deep.go"), []byte(""), 0o644)

	tree := Tree(dir, 2)

	if strings.Contains(tree, "deep.go") {
		t.Error("expected depth limit to hide deep.go at depth 2")
	}
}

func TestTreeIgnoresGitDir(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0o755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(""), 0o644)

	tree := Tree(dir, 4)

	if strings.Contains(tree, ".git") {
		t.Error("expected .git to be ignored")
	}
	if !strings.Contains(tree, "main.go") {
		t.Error("expected main.go to be present")
	}
}

func TestTreeIgnoresNodeModules(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "node_modules", "pkg"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.js"), []byte(""), 0o644)

	tree := Tree(dir, 4)

	if strings.Contains(tree, "node_modules") {
		t.Error("expected node_modules to be ignored")
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	content, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
}

func TestReadFileTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")

	// Create a file larger than 8KB
	data := strings.Repeat("x", 10000)
	os.WriteFile(path, []byte(data), 0o644)

	content, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(content, "truncated") {
		t.Error("expected truncation notice for large file")
	}
	if len(content) > 9000 {
		t.Errorf("expected content to be truncated, got %d bytes", len(content))
	}
}

func TestReadFileDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadFile(dir)
	if err == nil {
		t.Error("expected error reading directory")
	}
}

func TestReadFileNotExist(t *testing.T) {
	_, err := ReadFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestDetectRoot(t *testing.T) {
	dir := t.TempDir()
	// Resolve symlinks (macOS /var -> /private/var)
	dir, _ = filepath.EvalSymlinks(dir)

	subdir := filepath.Join(dir, "src", "pkg")
	os.MkdirAll(subdir, 0o755)

	// Create a go.mod at root
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0o644)

	// Change to subdir
	origDir, _ := os.Getwd()
	os.Chdir(subdir)
	defer os.Chdir(origDir)

	root, found := DetectRoot()
	if !found {
		t.Fatal("expected to find project root")
	}
	// Resolve symlinks on result too
	root, _ = filepath.EvalSymlinks(root)
	if root != dir {
		t.Errorf("expected root=%s, got %s", dir, root)
	}
}

func TestDetectRootNotFound(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	_, found := DetectRoot()
	if found {
		t.Error("expected no root found in temp dir without markers")
	}
}
