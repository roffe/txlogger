package snd

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
)

//go:embed pedro.mp3
var pedroMp3 []byte

func Pedro(otoCtx *oto.Context) error {
	fileBytesReader := bytes.NewReader(pedroMp3)
	decodedMp3, err := mp3.NewDecoder(fileBytesReader)
	if err != nil {
		return fmt.Errorf("mp3.NewDecoder failed: %w", err)
	}
	// Create a new 'player' that will handle our sound. Paused by default.
	player := otoCtx.NewPlayer(decodedMp3)

	// Play starts playing the sound and returns without waiting for it (Play() is async).
	player.Play()

	go func() {
		// We can wait for the sound to finish playing using something like this
		for player.IsPlaying() {
			time.Sleep(5 * time.Millisecond)
		}
		// Now that the sound finished playing, we can restart from the beginning (or go to any location in the sound) using seek
		// newPos, err := player.(io.Seeker).Seek(0, io.SeekStart)
		// if err != nil{
		//     panic("player.Seek failed: " + err.Error())
		// }
		// println("Player is now at position:", newPos)
		// player.Play()

		// If you don't want the player/sound anymore simply close
		err = player.Close()
		if err := player.Close(); err != nil {
			log.Printf("player.Close failed: %v", err)
		}
	}()
	return nil
}
