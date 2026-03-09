package native

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/sqweek/dialog"
)

// GenerateDBusToken generates a random D-Bus authentication token
func GenerateDBusToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "my_token_1" // fallback token in case of error
	}
	return hex.EncodeToString(b)
}

func buildPortalFilters(filters []FileFilter) dbus.Variant {
	type rule struct {
		Type    uint32
		Pattern string
	}
	type filterEntry struct {
		Name  string
		Rules []rule
	}
	entries := make([]filterEntry, 0, len(filters))
	for _, f := range filters {
		rules := make([]rule, 0, len(f.Extensions))
		for _, ext := range f.Extensions {
			ext = strings.TrimPrefix(ext, ".")
			pattern := "*." + ext
			if ext == "*" {
				pattern = "*"
			}
			rules = append(rules, rule{Type: 0, Pattern: pattern})
		}
		entries = append(entries, filterEntry{Name: f.Description, Rules: rules})
	}
	return dbus.MakeVariant(entries)
}

func portalCall(method string, options map[string]dbus.Variant, args ...interface{}) (string, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return "", fmt.Errorf("failed to connect to session bus: %w", err)
	}
	defer conn.Close()

	obj := conn.Object(
		"org.freedesktop.portal.Desktop",
		"/org/freedesktop/portal/desktop",
	)

	// Build full arg list: parent window, then caller args, then options
	callArgs := make([]interface{}, 0, 2+len(args))
	callArgs = append(callArgs, "") // parent window handle
	callArgs = append(callArgs, args...)
	callArgs = append(callArgs, options)

	var handle dbus.ObjectPath
	if err := obj.Call(method, 0, callArgs...).Store(&handle); err != nil {
		return "", fmt.Errorf("portal call %s failed: %w", method, err)
	}

	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(handle),
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return "", fmt.Errorf("failed to add match signal: %w", err)
	}

	c := make(chan *dbus.Signal, 10)
	conn.Signal(c)
	defer conn.RemoveSignal(c)

	for sig := range c {
		if sig.Path != handle || sig.Name != "org.freedesktop.portal.Request.Response" {
			continue
		}
		if len(sig.Body) < 2 {
			return "", errors.New("unexpected signal body")
		}
		response, ok := sig.Body[0].(uint32)
		if !ok {
			return "", fmt.Errorf("unexpected response type: %T", sig.Body[0])
		}
		if response != 0 {
			return "", ErrCancelled
		}
		results, ok := sig.Body[1].(map[string]dbus.Variant)
		if !ok {
			return "", fmt.Errorf("unexpected results type: %T", sig.Body[1])
		}
		uris, ok := results["uris"].Value().([]string)
		if !ok || len(uris) == 0 {
			return "", errors.New("no files selected")
		}
		return strings.TrimPrefix(uris[0], "file://"), nil
	}

	return "", errors.New("signal channel closed unexpectedly")
}

func OpenFileDialog(title string, filters ...FileFilter) (string, error) {
	options := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(GenerateDBusToken()),
		"multiple":     dbus.MakeVariant(false),
	}
	if len(filters) > 0 {
		options["filters"] = buildPortalFilters(filters)
	}
	filename, err := portalCall(
		"org.freedesktop.portal.FileChooser.OpenFile",
		options,
		title,
	)
	if err == nil {
		return filename, err
	}
	return dialog.File().Title("Open File").Filter("bin file", "bin").Load()
}

func OpenFolderDialog(title string) (string, error) {
	options := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(GenerateDBusToken()),
		"multiple":     dbus.MakeVariant(false),
		"directory":    dbus.MakeVariant(true),
	}
	filename, err := portalCall(
		"org.freedesktop.portal.FileChooser.OpenFile",
		options,
		title,
	)
	if err == nil {
		return filename, err
	}
	return dialog.File().Title("Open folder").Load()
}

func SaveFileDialog(title string, defaultExt string, filters ...FileFilter) (string, error) {
	options := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(GenerateDBusToken()),
	}
	if len(filters) > 0 {
		options["filters"] = buildPortalFilters(filters)
	}
	if defaultExt != "" {
		ext := strings.TrimPrefix(defaultExt, ".")
		// Find the matching filter from the provided list to use as current_filter.
		// If no match found, build a standalone one (only valid when filters is empty).
		type rule struct {
			Type    uint32
			Pattern string
		}
		type filterEntry struct {
			Name  string
			Rules []rule
		}
		var matched *FileFilter
		for _, f := range filters {
			for _, e := range f.Extensions {
				if strings.TrimPrefix(e, ".") == ext {
					matched = &f
					break
				}
			}
			if matched != nil {
				break
			}
		}
		if matched != nil {
			// Build current_filter from the matched filter entry so it's
			// identical to what was passed in filters — portal requires this.
			rules := make([]rule, 0, len(matched.Extensions))
			for _, e := range matched.Extensions {
				e = strings.TrimPrefix(e, ".")
				pattern := "*." + e
				if e == "*" {
					pattern = "*"
				}
				rules = append(rules, rule{Type: 0, Pattern: pattern})
			}
			options["current_filter"] = dbus.MakeVariant(filterEntry{
				Name:  matched.Description,
				Rules: rules,
			})
		} else if len(filters) == 0 {
			// No filters list at all — safe to set a standalone current_filter
			options["current_filter"] = dbus.MakeVariant(filterEntry{
				Name:  ext,
				Rules: []rule{{Type: 0, Pattern: "*." + ext}},
			})
		}
		// If filters is non-empty but no match found, skip current_filter entirely
		// to avoid the portal rejecting the call.
	}
	filename, err := portalCall(
		"org.freedesktop.portal.FileChooser.SaveFile",
		options,
		title,
	)
	if err == nil {
		return filename, err
	}
	return dialog.File().Title("Save File").Filter("bin file", "bin").Save()
}
