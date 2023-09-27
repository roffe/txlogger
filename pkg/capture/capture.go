package capture

import (
	"fmt"
	"image/jpeg"
	"log"
	"os"
	"time"

	"fyne.io/fyne/v2"
)

func Screenshot(c fyne.Canvas) {
	cap := c.Capture()
	filename := fmt.Sprintf("capture-%s.jpg", time.Now().Format("2006-01-02-15-04-05"))
	f, err := os.Create(filename)
	if err != nil {
		log.Println(err)
		return
	}
	defer f.Close()
	if err := jpeg.Encode(f, cap, &jpeg.Options{Quality: 95}); err != nil {
		log.Println(err)
		return
	}
	log.Println("Screenshot saved to", filename)
}
