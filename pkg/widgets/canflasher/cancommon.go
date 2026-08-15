package canflasher

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/roffe/txlogger/pkg/ecu"
)

// ecuConfig builds the config every flasher action shares. Call it on the main
// thread — it reads widget state.
func (t *CanFlasherWidget) ecuConfig() *ecu.Config {
	return &ecu.Config{
		Name:       t.ecuSelect.Selected,
		SeedKey:    t.seedKey(),
		OnProgress: t.progress,
		OnMessage:  t.log,
		OnError:    func(err error) { t.log(err.Error()) },
	}
}

// seedKey returns the user-entered T7 XOR/SUB pair, or nil when either box is
// empty or not valid hex — then the known-good pairs are used.
func (t *CanFlasherWidget) seedKey() *ecu.SeedKey {
	xor, err1 := parseHex16(t.seedKeyXOR.Text)
	sub, err2 := parseHex16(t.seedKeySub.Text)
	if err1 != nil || err2 != nil {
		return nil
	}
	return &ecu.SeedKey{XOR: xor, Sub: sub}
}

func parseHex16(s string) (uint16, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	v, err := strconv.ParseUint(s, 16, 16)
	return uint16(v), err
}

func showIf(cond bool, objs ...fyne.CanvasObject) {
	for _, o := range objs {
		if cond {
			o.Show()
		} else {
			o.Hide()
		}
	}
}

func addSuffix(s, suffix string) string {
	if !strings.HasSuffix(s, suffix) {
		return s + suffix
	}
	return s
}

/*
func translateName(s string) string {
	switch s {
	case "T5":
		return "Trionic 5"
	case "T7":
		return "Trionic 7"
	case "T8":
		return "Trionic 8"
	default:
		return "Unknown"
	}
}
*/
