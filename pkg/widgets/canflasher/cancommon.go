package canflasher

import "strings"

func addSuffix(s, suffix string) string {
	if !strings.HasSuffix(s, suffix) {
		return s + suffix
	}
	return s
}

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
