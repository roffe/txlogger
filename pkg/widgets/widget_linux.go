package widgets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/roffe/browse"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

// dialogChildEnv is set for a copy of the program started to show one file
// dialog.
const dialogChildEnv = "FP"

type dialogRequest struct {
	Op      string
	Options browse.Options
}

type dialogResponse struct {
	Paths     []string
	Cancelled bool
	Err       string
}

// dialog shows the dialog from a child process that has given up
// CAP_NET_ADMIN, see RunFileChild.
func dialog(op string, o browse.Options) ([]string, error) {
	req, err := json.Marshal(dialogRequest{Op: op, Options: o})
	if err != nil {
		return nil, err
	}
	child := exec.Command("/proc/self/exe") // re-exec self
	child.Env = append(os.Environ(), dialogChildEnv+"=1")
	child.Stdin = bytes.NewReader(req)
	child.Stderr = os.Stderr
	out, err := child.Output()
	if err != nil {
		return nil, fmt.Errorf("file dialog child: %w", err)
	}
	var resp dialogResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("file dialog child: %w", err)
	}
	switch {
	case resp.Cancelled:
		return nil, browse.ErrCancelled
	case resp.Err != "":
		return nil, errors.New(resp.Err)
	}
	return resp.Paths, nil
}

// RunFileChild reports whether this process was started by dialog. If it was,
// the dialog has been shown and answered on stdout, and the program should
// exit.
func RunFileChild() bool {
	if os.Getenv(dialogChildEnv) != "1" {
		return false
	}
	if err := dropNetAdmin(); err != nil {
		log.Fatalf("failed to drop NET_ADMIN capability: %v", err)
	}
	var req dialogRequest
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		log.Fatalf("error decoding request: %v", err)
	}
	resp := dialogResponse{}
	var err error
	if resp.Paths, err = showDialog(req.Op, req.Options); err != nil {
		resp.Cancelled = errors.Is(err, browse.ErrCancelled)
		resp.Err = err.Error()
	}
	if err := json.NewEncoder(os.Stdout).Encode(resp); err != nil {
		log.Fatalf("error encoding response: %v", err)
	}
	return true
}

// dropNetAdmin removes CAP_NET_ADMIN from the effective, inheritable and
// permitted sets of the process.
func dropNetAdmin() error {
	caps, err := cap.GetPID(0) // 0 = current process
	if err != nil {
		return fmt.Errorf("get caps: %w", err)
	}
	for _, flag := range []cap.Flag{cap.Effective, cap.Inheritable, cap.Permitted} {
		if err := caps.SetFlag(flag, false, cap.NET_ADMIN); err != nil {
			return fmt.Errorf("drop cap: %w", err)
		}
	}
	if err := caps.SetProc(); err != nil {
		return fmt.Errorf("set caps: %w", err)
	}
	return nil
}
