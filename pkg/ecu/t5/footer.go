package t5

import (
	"context"
	"fmt"
	"log"
	"strings"
)

func (t *Client) GetECUFooter(ctx context.Context) ([]byte, error) {
	if len(t.footer) > 0 {
		return t.footer, nil
	}

	footer, err := t.readFlash(ctx, 0x7FF80, 0x80)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECU footer: %w", err)
	}

	t.footer = footer
	return footer, nil
}

// invalidateFooter must be called whenever the flash content changes
// (erase/flash) so a stale footer is never used afterwards.
func (t *Client) invalidateFooter() {
	t.footer = nil
}

func GetIdentifierFromFooter(footer []byte, identifier byte) string {
	var result strings.Builder
	offset := len(footer) - 0x05 //  avoid the stored checksum
	for offset > 0 {
		length := int(footer[offset])
		offset--
		search := footer[offset]
		offset--
		if offset < 0 || length > offset+1 {
			break // corrupt footer, entry would run past the start
		}
		if identifier == search {
			for i := 0; i < length; i++ {
				result.WriteByte(footer[offset])
				offset--
			}
			return result.String()
		}
		offset -= length
	}
	log.Printf("error getting identifier 0x%X", identifier)
	return ""
}
