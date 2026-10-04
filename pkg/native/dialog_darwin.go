package native

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers

#import <Cocoa/Cocoa.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>

enum { panelOpen, panelFolder, panelSave };

// exts is a comma separated list of file extensions, empty for no restriction.
static NSSavePanel* makePanel(int mode, NSString* title, NSString* exts, bool multi) {
	NSSavePanel* panel;
	if (mode == panelSave) {
		panel = [NSSavePanel savePanel];
	} else {
		NSOpenPanel* open = [NSOpenPanel openPanel];
		open.canChooseFiles = mode == panelOpen;
		open.canChooseDirectories = mode == panelFolder;
		open.allowsMultipleSelection = multi;
		panel = open;
	}
	panel.message = title;
	panel.canCreateDirectories = YES;

	NSMutableArray<UTType*>* types = [NSMutableArray array];
	for (NSString* ext in [exts componentsSeparatedByString:@","]) {
		UTType* type = ext.length ? [UTType typeWithFilenameExtension:ext] : nil;
		if (type) [types addObject:type];
	}
	if (types.count) panel.allowedContentTypes = types;
	return panel;
}

// panelPaths returns the chosen paths as a malloc'd JSON array.
static char* panelPaths(NSSavePanel* panel) {
	NSArray<NSURL*>* urls = [panel isKindOfClass:[NSOpenPanel class]] ? ((NSOpenPanel*)panel).URLs : @[panel.URL];
	NSData* json = [NSJSONSerialization dataWithJSONObject:[urls valueForKey:@"path"] options:0 error:nil];
	return strndup(json.bytes, json.length);
}

// runPanel blocks until the panel is dismissed and returns NULL if it was
// cancelled. AppKit only runs on the main thread, so from any other thread the
// panel is opened non-modal on the main queue and the event loop keeps running
// while we wait. On the main thread itself it runs modal.
static char* runPanel(int mode, const char* ctitle, const char* cexts, bool multi) {
	@autoreleasepool {
		NSString* title = [NSString stringWithUTF8String:ctitle];
		NSString* exts = [NSString stringWithUTF8String:cexts];

		if ([NSThread isMainThread]) {
			NSSavePanel* panel = makePanel(mode, title, exts, multi);
			return [panel runModal] == NSModalResponseOK ? panelPaths(panel) : NULL;
		}

		__block char* result = NULL;
		dispatch_semaphore_t done = dispatch_semaphore_create(0);
		dispatch_async(dispatch_get_main_queue(), ^{
			NSSavePanel* panel = makePanel(mode, title, exts, multi);
			[panel beginWithCompletionHandler:^(NSModalResponse response) {
				if (response == NSModalResponseOK) result = panelPaths(panel);
				dispatch_semaphore_signal(done);
			}];
		});
		dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
		return result;
	}
}
*/
import "C"

import (
	"encoding/json"
	"strings"
	"unsafe"
)

func panel(mode C.int, title string, multi bool, exts []string) ([]string, error) {
	ctitle, cexts := C.CString(title), C.CString(strings.Join(exts, ","))
	defer C.free(unsafe.Pointer(ctitle))
	defer C.free(unsafe.Pointer(cexts))

	res := C.runPanel(mode, ctitle, cexts, C.bool(multi))
	if res == nil {
		return nil, ErrCancelled
	}
	defer C.free(unsafe.Pointer(res))

	var paths []string
	if err := json.Unmarshal([]byte(C.GoString(res)), &paths); err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, ErrCancelled
	}
	return paths, nil
}

func panelOne(mode C.int, title string, exts []string) (string, error) {
	paths, err := panel(mode, title, false, exts)
	if err != nil {
		return "", err
	}
	return paths[0], nil
}

// panelExts flattens the filters into a list of extensions, nil means no
// restriction.
func panelExts(filters []FileFilter) []string {
	var exts []string
	for _, f := range filters {
		for _, ext := range f.Extensions {
			ext = strings.TrimPrefix(ext, ".")
			if ext == "" || ext == "*" {
				return nil
			}
			exts = append(exts, ext)
		}
	}
	return exts
}

func OpenFileDialog(title string, filters ...FileFilter) (string, error) {
	return panelOne(C.panelOpen, title, panelExts(filters))
}

func OpenFilesDialog(title string, filters ...FileFilter) ([]string, error) {
	return panel(C.panelOpen, title, true, panelExts(filters))
}

func OpenFolderDialog(title string) (string, error) {
	return panelOne(C.panelFolder, title, nil)
}

// SaveFileDialog appends defaultExt to the chosen name when it is missing.
func SaveFileDialog(title string, defaultExt string, filters ...FileFilter) (string, error) {
	exts := panelExts(filters)
	if ext := strings.TrimPrefix(defaultExt, "."); ext != "" {
		exts = []string{ext}
	}
	return panelOne(C.panelSave, title, exts)
}
