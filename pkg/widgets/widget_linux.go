package widgets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/roffe/txlogger/pkg/native"
)

func selectFile(desc string, exts ...string) (string, error) {
	return runChild("open_file", "Open "+desc, desc, exts...)
}

func saveFile(desc string, ext string) (string, error) {
	return runChild("save_file", "Save "+desc, desc, ext)
}

func selectFolder() (string, error) {
	return runChild("select_folder", "Select folder", "", "")
}

func runChild(op, title, desc string, exts ...string) (string, error) {
	child := exec.Command("/proc/self/exe") // re-exec self
	child.Env = append(os.Environ(), "FP=1")
	childIn, _ := child.StdinPipe()
	childOut, _ := child.StdoutPipe()
	child.Stderr = os.Stderr
	defer childIn.Close()

	if err := child.Start(); err != nil {
		return "", fmt.Errorf("failed to start child: %w\n", err)
	}

	enc := json.NewEncoder(childIn)
	dec := json.NewDecoder(childOut)

	req := native.FileRequest{
		Op:    op,
		Title: title,
		Desc:  desc,
		Exts:  exts,
	}
	if err := enc.Encode(req); err != nil {
		return "", fmt.Errorf("error decoding response: %w", err)
	}

	var resp native.FileResponse
	decodeErr := dec.Decode(&resp)

	waitErr := child.Wait()

	if decodeErr != nil {
		return "", decodeErr
	}

	if resp.Err != "" {
		return resp.Path, errors.New(resp.Err)
	}

	return resp.Path, waitErr
}
