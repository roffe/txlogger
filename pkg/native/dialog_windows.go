package native

import (
	"fmt"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comdlg32             = windows.NewLazySystemDLL("comdlg32.dll")
	procGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
	procGetSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")

	shell32               = windows.NewLazySystemDLL("shell32.dll")
	procSHBrowseForFolder = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromID   = shell32.NewProc("SHGetPathFromIDListW")

	ole32              = windows.NewLazySystemDLL("ole32.dll")
	procCoInitialize   = ole32.NewProc("CoInitialize")
	procCoUninitialize = ole32.NewProc("CoUninitialize")
)

const (
	MAX_PATH            = 260
	OFN_EXPLORER        = 0x00080000
	OFN_FILEMUSTEXIST   = 0x00001000
	OFN_PATHMUSTEXIST   = 0x00000800
	OFN_OVERWRITEPROMPT = 0x00000002
)

type openfilenameW struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	Flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	FlagsEx           uint32
}

type FileFilter struct {
	Description string
	Extensions  []string
}

func OpenFileDialog(title string, filters ...FileFilter) (string, error) {
	// Buffer for the returned path (must be preallocated)
	fileBuf := make([]uint16, MAX_PATH)

	// Filter string: pairs of (label, pattern) with NUL separators and a final double-NUL.
	// Example: "Text files\x00*.txt\x00All files\x00*.*\x00\x00"

	var filter []uint16
	for _, filt := range filters {
		desc := fmt.Sprintf("%s (%s)", filt.Description, strings.Join(filt.Extensions, ","))
		filter = append(filter, utf16.Encode([]rune(desc))...)
		filter = append(filter, 0x00)
		for _, ext := range filt.Extensions {
			s := fmt.Sprintf("*.%s;", ext)
			filter = append(filter, utf16.Encode([]rune(s))...)
		}
		filter = append(filter, 0x00)
	}

	filterPtr := utf16ptr(filter)
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return "", err
	}

	ofn := openfilenameW{
		lStructSize:  uint32(unsafe.Sizeof(openfilenameW{})),
		lpstrFilter:  filterPtr,
		lpstrFile:    &fileBuf[0],
		nMaxFile:     MAX_PATH,
		lpstrTitle:   titlePtr,
		Flags:        OFN_EXPLORER | OFN_FILEMUSTEXIST | OFN_PATHMUSTEXIST,
		nFilterIndex: 1, // 1-based index into filter pairs
	}

	ret, _, err := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		// If the user cancels, GetOpenFileNameW returns 0 and CommDlgExtendedError() could be used.
		// We’ll just surface the syscall error here.
		if err != syscall.Errno(0) {
			return "", err
		}
		return "", ErrCancelled
	}

	return windows.UTF16PtrToString(ofn.lpstrFile), nil
}

type browseinfoW struct {
	HwndOwner      uintptr
	PidlRoot       uintptr
	PszDisplayName *uint16
	LpszTitle      *uint16
	UlFlags        uint32
	Lpfn           uintptr
	LParam         uintptr
	IImage         int32
}

// Flags for SHBrowseForFolder
const (
	BIF_RETURNONLYFSDIRS   = 0x00000001
	BIF_NEWDIALOGSTYLE     = 0x00000040
	BIF_EDITBOX            = 0x00000010
	BIF_USENEWUI           = BIF_NEWDIALOGSTYLE | BIF_EDITBOX
	BIF_VALIDATE           = 0x00000020
	BIF_BROWSEINCLUDEFILES = 0x00004000
)

func OpenFolderDialog(title string) (string, error) {
	// Initialize COM (required on some systems)
	procCoInitialize.Call(0)
	defer procCoUninitialize.Call()

	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return "", err
	}

	displayName := make([]uint16, windows.MAX_PATH)

	bi := browseinfoW{
		HwndOwner:      0,
		PidlRoot:       0,
		PszDisplayName: &displayName[0],
		LpszTitle:      titlePtr,
		UlFlags:        BIF_RETURNONLYFSDIRS | BIF_USENEWUI,
		Lpfn:           0,
		LParam:         0,
		IImage:         0,
	}

	ret, _, _ := procSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if ret == 0 {
		return "", ErrCancelled
	}

	var pathBuf [windows.MAX_PATH]uint16
	ok, _, _ := procSHGetPathFromID.Call(ret, uintptr(unsafe.Pointer(&pathBuf[0])))
	if ok == 0 {
		return "", fmt.Errorf("failed to get folder path")
	}

	return syscall.UTF16ToString(pathBuf[:]), nil
}

// SaveFileDialog shows a native “Save As” dialog and returns the chosen path.
func SaveFileDialog(title string, defaultExt string, filters ...FileFilter) (string, error) {
	// Output buffer (preallocated)
	fileBuf := make([]uint16, MAX_PATH)

	// Build filter pairs (label, pattern) with NUL separators and final double-NUL.
	// Example (UTF-16): "Text files\x00*.txt\x00All files\x00*.*\x00\x00"
	var filter []uint16
	if len(filters) > 0 {
		for _, f := range filters {
			filter = append(filter, utf16.Encode([]rune(f.Description))...)
			filter = append(filter, 0)
			// Join multiple extensions with ';' (e.g., "*.log;*.txt")
			var pat []rune
			for i, ext := range f.Extensions {
				if i > 0 {
					pat = append(pat, []rune(";")...)
				}
				pat = append(pat, []rune("*."+ext)...)
			}
			if len(pat) == 0 {
				// Fallback to all files if no extensions provided in this filter
				pat = []rune("*.*")
			}
			filter = append(filter, utf16.Encode(pat)...)
			filter = append(filter, 0)
		}
		// Final double-NUL to end the whole filter list
		filter = append(filter, 0)
	}

	var filterPtr *uint16
	if len(filter) > 0 {
		filterPtr = &filter[0]
	}

	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return "", err
	}

	var defExtPtr *uint16
	if defaultExt != "" {
		// Windows expects just the extension without dot (e.g., "txt")
		defExtPtr, err = windows.UTF16PtrFromString(defaultExt)
		if err != nil {
			return "", err
		}
	}

	ofn := openfilenameW{
		lStructSize:  uint32(unsafe.Sizeof(openfilenameW{})),
		lpstrFilter:  filterPtr,
		lpstrFile:    &fileBuf[0],
		nMaxFile:     MAX_PATH,
		lpstrTitle:   titlePtr,
		lpstrDefExt:  defExtPtr,
		Flags:        OFN_EXPLORER | OFN_OVERWRITEPROMPT | OFN_PATHMUSTEXIST,
		nFilterIndex: 1,
	}

	ret, _, callErr := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret == 0 {
		// User canceled (or error). If callErr is non-zero, it’s a Windows error.
		if callErr != syscall.Errno(0) {
			return "", callErr
		}
		return "", fmt.Errorf("dialog canceled")
	}

	return windows.UTF16PtrToString(ofn.lpstrFile), nil
}
