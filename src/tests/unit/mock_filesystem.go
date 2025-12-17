package unit

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MockFileSystem implementa a interface FileSystem para testes
type MockFileSystem struct {
	files       map[string][]byte
	dirs        map[string]bool
	writeError  error
	readError   error
	statError   error
	createError error
	openError   error
}

func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (m *MockFileSystem) SetFile(path string, content []byte) {
	m.files[path] = content
}

func (m *MockFileSystem) SetDir(path string) {
	m.dirs[path] = true
}

func (m *MockFileSystem) SetWriteError(err error) {
	m.writeError = err
}

func (m *MockFileSystem) SetReadError(err error) {
	m.readError = err
}

func (m *MockFileSystem) SetStatError(err error) {
	m.statError = err
}

func (m *MockFileSystem) SetCreateError(err error) {
	m.createError = err
}

func (m *MockFileSystem) SetOpenError(err error) {
	m.openError = err
}

func (m *MockFileSystem) GetFile(path string) ([]byte, bool) {
	content, exists := m.files[path]
	return content, exists
}

func (m *MockFileSystem) FileExists(path string) bool {
	_, exists := m.files[path]
	return exists
}

func (m *MockFileSystem) DirExists(path string) bool {
	return m.dirs[path]
}

func (m *MockFileSystem) ReadFile(filename string) ([]byte, error) {
	if m.readError != nil {
		return nil, m.readError
	}

	if content, exists := m.files[filename]; exists {
		return content, nil
	}

	return nil, &fs.PathError{Op: "read", Path: filename, Err: fs.ErrNotExist}
}

func (m *MockFileSystem) WriteFile(filename string, data []byte, perm fs.FileMode) error {
	if m.writeError != nil {
		return m.writeError
	}

	m.files[filename] = data
	return nil
}

func (m *MockFileSystem) Stat(filename string) (fs.FileInfo, error) {
	if m.statError != nil {
		return nil, m.statError
	}

	if _, exists := m.files[filename]; exists {
		return &mockFileInfo{name: filepath.Base(filename), size: int64(len(m.files[filename])), isDir: false}, nil
	}

	if m.dirs[filename] {
		return &mockFileInfo{name: filepath.Base(filename), size: 0, isDir: true}, nil
	}

	return nil, &fs.PathError{Op: "stat", Path: filename, Err: fs.ErrNotExist}
}

func (m *MockFileSystem) Create(filename string) (*os.File, error) {
	if m.createError != nil {
		return nil, m.createError
	}

	m.files[filename] = []byte{}
	return nil, nil
}

func (m *MockFileSystem) OpenFile(filename string, flag int, perm fs.FileMode) (*os.File, error) {
	if m.openError != nil {
		return nil, m.openError
	}

	if _, exists := m.files[filename]; !exists && (flag&os.O_CREATE) == 0 {
		return nil, &fs.PathError{Op: "open", Path: filename, Err: fs.ErrNotExist}
	}

	if _, exists := m.files[filename]; !exists && (flag&os.O_CREATE) != 0 {
		m.files[filename] = []byte{}
	}

	return nil, nil
}

func (m *MockFileSystem) ReadDir(dirname string) ([]fs.DirEntry, error) {
	if m.readError != nil {
		return nil, m.readError
	}

	// Normalize the directory path for comparison - use ToSlash for cross-platform compatibility
	normalizedDir := filepath.ToSlash(filepath.Clean(dirname))
	dirExists := false
	for d := range m.dirs {
		if filepath.ToSlash(filepath.Clean(d)) == normalizedDir {
			dirExists = true
			break
		}
	}
	if !dirExists {
		// Check if any files are in this directory
		hasFiles := false
		for f := range m.files {
			if filepath.ToSlash(filepath.Clean(filepath.Dir(f))) == normalizedDir {
				hasFiles = true
				break
			}
		}
		if !hasFiles {
			return nil, &fs.PathError{Op: "readdir", Path: dirname, Err: fs.ErrNotExist}
		}
	}

	var entries []fs.DirEntry

	// normalizedDir is already normalized with ToSlash
	normalizedDirSlash := normalizedDir

	for path := range m.files {
		normalizedPathDir := filepath.ToSlash(filepath.Clean(filepath.Dir(path)))
		if normalizedPathDir == normalizedDirSlash {
			entries = append(entries, &mockDirEntry{
				name:  filepath.Base(path),
				isDir: false,
			})
		}
	}

	for path := range m.dirs {
		normalizedPath := filepath.ToSlash(filepath.Clean(path))
		normalizedPathDir := filepath.ToSlash(filepath.Clean(filepath.Dir(path)))
		if normalizedPathDir == normalizedDirSlash && normalizedPath != normalizedDirSlash {
			entries = append(entries, &mockDirEntry{
				name:  filepath.Base(path),
				isDir: true,
			})
		}
	}

	return entries, nil
}

func (m *MockFileSystem) Remove(filename string) error {
	if m.writeError != nil {
		return m.writeError
	}

	if _, exists := m.files[filename]; exists {
		delete(m.files, filename)
		return nil
	}

	if m.dirs[filename] {
		delete(m.dirs, filename)
		return nil
	}

	return &fs.PathError{Op: "remove", Path: filename, Err: fs.ErrNotExist}
}

func (m *MockFileSystem) Mkdir(dirname string, perm fs.FileMode) error {
	if m.writeError != nil {
		return m.writeError
	}

	if m.dirs[dirname] {
		return &fs.PathError{Op: "mkdir", Path: dirname, Err: fs.ErrExist}
	}

	m.dirs[dirname] = true
	return nil
}

func (m *MockFileSystem) MkdirAll(dirname string, perm fs.FileMode) error {
	if m.writeError != nil {
		return m.writeError
	}

	parts := strings.Split(dirname, string(filepath.Separator))
	current := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if current == "" {
			current = part
		} else {
			current = filepath.Join(current, part)
		}
		m.dirs[current] = true
	}
	m.dirs[dirname] = true

	return nil
}

type mockFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() fs.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() interface{}   { return nil }

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string      { return m.name }
func (m *mockDirEntry) IsDir() bool       { return m.isDir }
func (m *mockDirEntry) Type() fs.FileMode { return 0644 }
func (m *mockDirEntry) Info() (fs.FileInfo, error) {
	return &mockFileInfo{name: m.name, isDir: m.isDir}, nil
}
