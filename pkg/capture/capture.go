package capture

import (
	"bytes"
	"image/png"
	"log"

	"fyne.io/fyne/v2"
	"golang.design/x/clipboard"
)

func Screenshot(c fyne.Canvas) {
	cap := c.Capture()
	//filename := fmt.Sprintf("capture-%s.jpg", time.Now().Format("2006-01-02-15-04-05"))
	//f, err := os.Create(filename)
	//if err != nil {
	//	log.Println(err)
	//	return
	//}
	//defer f.Close()
	buff := bytes.NewBuffer(nil)
	if err := png.Encode(buff, cap); err != nil {
		log.Println(err)
		return
	}
	log.Println("Copied to clipboard")
	clipboard.Write(clipboard.FmtImage, buff.Bytes())
}
