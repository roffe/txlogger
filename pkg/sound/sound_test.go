package sound

import "testing"

// Suspend before any sound has played is a no-op: it does not open the audio
// device to suspend it.
func TestSuspendWithoutContext(t *testing.T) {
	if octx != nil {
		t.Fatal("audio context already initialised")
	}
	Suspend()
	if octx != nil {
		t.Fatal("Suspend created an audio context")
	}
}
