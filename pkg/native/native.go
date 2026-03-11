package native

import (
	"errors"
)

var (
	ErrCancelled = errors.New("Cancelled")
)

// FileFilter describes a file filter for file dialogs.
type FileFilter struct {
	Description string
	Extensions  []string
}

type FileRequest struct {
	Op    string
	Title string
	Desc  string
	Exts  []string
}

type FileResponse struct {
	Path string
	Err  string
}
