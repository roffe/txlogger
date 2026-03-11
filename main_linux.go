package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/roffe/txlogger/pkg/native"
	"kernel.org/pub/linux/libs/security/libcap/cap"
)

func runFileChild() {
	_, err := native.Drop(cap.NET_ADMIN)
	if err != nil {
		log.Fatalf("failed to drop NET_ADMIN capability: %v", err)
	}
	dec := json.NewDecoder(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	var req native.FileRequest
	if err := dec.Decode(&req); err != nil {
		log.Printf("error decoding request: %v", err)
		return
	}

	var path string
	switch req.Op {
	case "select_folder":
		path, err = native.OpenFolderDialog(req.Title)
	case "save_file":
		path, err = native.SaveFileDialog(req.Title, req.Exts[0], native.FileFilter{
			Description: req.Desc,
			Extensions:  req.Exts,
		})
	case "open_file":
		path, err = native.OpenFileDialog(req.Title, native.FileFilter{
			Description: req.Desc,
			Extensions:  req.Exts,
		})
	case "quit":
		return
	default:
		log.Printf("unknown operation: %s", req.Op)
		return
	}

	resp := native.FileResponse{Path: path}
	if err != nil {
		resp.Err = err.Error()
	}

	if err := enc.Encode(resp); err != nil {
		log.Printf("error encoding response: %v", err)
	}
}
