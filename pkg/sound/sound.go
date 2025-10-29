package sound

import (
	"fmt"
	"io"
	"time"

	"github.com/ebitengine/oto/v3"
)

var octx *oto.Context

func Init() error {
	// Prepare an Oto context (this will use your default audio device) that will
	// play all our sounds. Its configuration can't be changed later.

	op := &oto.NewContextOptions{}

	// Usually 44100 or 48000. Other values might cause distortions in Oto
	op.SampleRate = 44100

	// Number of channels (aka locations) to play sounds from. Either 1 or 2.
	// 1 is mono sound, and 2 is stereo (most speakers are stereo).
	op.ChannelCount = 2

	// Format of the source. go-mp3's format is signed 16bit integers.
	op.Format = oto.FormatSignedInt16LE

	// Remember that you should **not** create more than one context
	otoCtx, readyChan, err := oto.NewContext(op)
	if err != nil {
		return fmt.Errorf("sound.Init failed: %w", err)
	}
	// It might take a bit for the hardware audio devices to be ready, so we wait on the channel.
	select {
	case <-readyChan:
		octx = otoCtx
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("sound.Init timed out")
	}
}

// Create a new 'player' that will handle our sound. Paused by default.
func NewPlayer(r io.Reader) *oto.Player {
	if octx == nil {
		if err := Init(); err != nil {
			panic("sound.NewPlayer: " + err.Error())
		}
	}
	return octx.NewPlayer(r)
}
