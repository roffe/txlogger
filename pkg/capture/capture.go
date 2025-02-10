package capture

import (
	"bytes"
	"fmt"
	"image/png"
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
	buff := bytes.NewBuffer(nil)
	if err := png.Encode(buff, cap); err != nil {
		log.Println(err)
		return
	}
	if _, err := f.Write(buff.Bytes()); err != nil {
		log.Println(err)
		return
	}
}
