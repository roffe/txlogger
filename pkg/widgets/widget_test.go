package widgets

import (
	"errors"
	"testing"

	"github.com/roffe/browse"
)

// showDialog rejects an operation it does not know without showing anything.
func TestShowDialogUnknownOp(t *testing.T) {
	paths, err := showDialog("print", browse.Options{Title: "t"})
	if err == nil || err.Error() != `unknown file dialog operation "print"` {
		t.Errorf("error %v, want the unknown operation", err)
	}
	if errors.Is(err, browse.ErrCancelled) {
		t.Error("unknown operation reported as a cancel")
	}
	if paths != nil {
		t.Errorf("paths %q, want none", paths)
	}
}
