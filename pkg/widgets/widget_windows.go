package widgets

import "github.com/roffe/txlogger/pkg/native"

func selectFile(desc string, exts ...string) (string, error) {
	filter := native.FileFilter{Description: desc, Extensions: exts}
	return native.OpenFileDialog("Open file", filter)
}

func saveFile(desc string, ext string) (string, error) {
	return native.SaveFileDialog("Save "+desc, ext, native.FileFilter{
		Description: desc,
		Extensions:  []string{ext},
	})
}

func selectFolder() (string, error) {
	return native.OpenFolderDialog("Select log folder")
}
