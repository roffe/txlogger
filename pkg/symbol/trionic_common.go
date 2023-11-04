package symbol

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/roffe/txlogger/pkg/blowfish"
	"github.com/roffe/txlogger/pkg/lzhuf"
)

func ExpandCompressedSymbolNames(in []byte) ([]string, error) {
	if len(in) < 0x1000 {
		return nil, errors.New("invalid symbol table size")
	}
	//os.WriteFile("compressedSymbolTable.bin", in, 0644)
	if bytes.HasPrefix(in, []byte{0xF1, 0x1A, 0x06, 0x5B, 0xA2, 0x6B, 0xCC, 0x6F}) {
		return blowfish.DecryptSymbolNames(in)
	}

	expandedFileSize := int(in[0]) | (int(in[1]) << 8) | (int(in[2]) << 16) | (int(in[3]) << 24)

	if expandedFileSize == -1 {
		return nil, errors.New("invalid expanded file size")
	}

	out := make([]byte, expandedFileSize)
	returnedSize := lzhuf.Decode(in, out)

	if returnedSize != expandedFileSize {
		return nil, fmt.Errorf("decoded data size missmatch: %d != %d", returnedSize, expandedFileSize)
	}

	return strings.Split(strings.TrimSuffix(string(out), "\r\n"), "\r\n"), nil
}
