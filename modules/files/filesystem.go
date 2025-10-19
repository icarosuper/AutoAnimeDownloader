package files

import (
	"io/fs"
	"os"
)

type FileSystem interface {
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm fs.FileMode) error
	Stat(filename string) (fs.FileInfo, error)
	Create(filename string) (*os.File, error)
	OpenFile(filename string, flag int, perm fs.FileMode) (*os.File, error)
	ReadDir(dirname string) ([]fs.DirEntry, error)
	Remove(filename string) error
	Mkdir(dirname string, perm fs.FileMode) error
	MkdirAll(dirname string, perm fs.FileMode) error
}

type OSFileSystem struct{}

func NewOSFileSystem() *OSFileSystem {
	return &OSFileSystem{}
}

func (osfs *OSFileSystem) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (osfs *OSFileSystem) WriteFile(filename string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (osfs *OSFileSystem) Stat(filename string) (fs.FileInfo, error) {
	return os.Stat(filename)
}

func (osfs *OSFileSystem) Create(filename string) (*os.File, error) {
	return os.Create(filename)
}

func (osfs *OSFileSystem) OpenFile(filename string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(filename, flag, perm)
}

func (osfs *OSFileSystem) ReadDir(dirname string) ([]fs.DirEntry, error) {
	return os.ReadDir(dirname)
}

func (osfs *OSFileSystem) Remove(filename string) error {
	return os.Remove(filename)
}

func (osfs *OSFileSystem) Mkdir(dirname string, perm fs.FileMode) error {
	return os.Mkdir(dirname, perm)
}

func (osfs *OSFileSystem) MkdirAll(dirname string, perm fs.FileMode) error {
	return os.MkdirAll(dirname, perm)
}
