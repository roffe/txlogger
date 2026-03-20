package t8file

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/ecu/t8util"
)

var T8MagicBytes = []byte{0x00, 0x10, 0x0C, 0x00}

type T8File struct {
	filePath string
	data     []byte
	th       *T8Header
}

func (tf *T8File) GetInfo(path string) *T8Header {
	tf.data, _ = os.ReadFile(path)
	tf.filePath = path
	if tf.th == nil {
		tf.th = new(T8Header)
	}
	if isValidT8Bin(tf.data) {
		tf.th.DecodeInfo(tf.data)
		tf.th.DecodeExtraInfo(tf.data)
	}
	return tf.th
}

func (tf *T8File) saveDataToBinary(address int64, length int, data []byte) error {

	file, err := os.OpenFile(tf.filePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Seek(address, 0)
	if err != nil {
		return err
	}

	toWrite := data
	if len(toWrite) > length {
		toWrite = toWrite[:length]
	}

	_, err = file.Write(toWrite)
	return err
}

func (tf *T8File) SaveFlashBlock() error {
	tf.th.EncodeExtraInfo()

	for _, fb := range tf.th.fbc {
		if err := tf.saveDataToBinary(int64(fb.BlockAddress), len(fb.BlockData), fb.BlockData); err != nil {
			return err
		}
	}
	return nil
}

func GetChecksumAreaOffsetFromBytes(data []byte) uint32 {
	offset := 0x20140

	if len(data) < offset+4 {
		return 0
	}

	bytes := data[offset : offset+4]

	return binary.BigEndian.Uint32(bytes)
}

func GetEmptySpaceStartFrom(data []byte, offset int) int {
	if len(data) == 0 || offset >= len(data) {
		return 0
	}

	limit := offset + 0x1000
	if limit > len(data)-3 {
		limit = len(data) - 2
	}

	for i := offset; i < limit; i++ {
		if data[i] == 0xFF && data[i+1] == 0xFF && data[i+2] == 0xFF {
			return i
		}
	}

	return limit
}

func readDataFromBytes(data []byte, address int, length int) []byte {
	retval := make([]byte, length)
	dataLen := len(data)
	if dataLen == 0 {
		return retval
	}
	actualAddress := address % dataLen
	copyLen := length
	if actualAddress+length > dataLen {
		copyLen = dataLen - actualAddress
	}
	copy(retval, data[actualAddress:actualAddress+copyLen])

	return retval
}

func isValidT8Bin(data []byte) bool {
	if len(data) < int(t8util.T8binSize) {
		return false
	}

	return bytes.HasPrefix(data, T8MagicBytes)
}

func (tf *T8File) ShowEditT8Dialog(win fyne.Window) {
	vinEntry := widget.NewEntry()
	vinEntry.SetText(tf.th.VIN())
	vinEntry.Validator = func(s string) error {
		if len(s) != 17 {
			return errors.New("VIN must be 17 characters long")
		}
		return nil
	}

	ecuEntry := widget.NewEntry()
	ecuEntry.SetText(tf.th.EcuDescription())
	ecuEntry.Validator = func(s string) error {
		if len(s) != 16 {
			return errors.New("EcuDescription must be 16 characters long")
		}
		return nil
	}

	partEntry := widget.NewEntry()
	partEntry.SetText(tf.th.PartNumber())
	partEntry.Validator = func(s string) error {
		if len(s) != 8 {
			return errors.New("PartNumber must be 8 digits long")
		}
		return nil
	}

	softEntry := widget.NewEntry()
	softEntry.SetText(tf.th.SoftwareVersion())
	softEntry.Validator = func(s string) error {
		if len(s) != 30 {
			return errors.New("SoftwareVersion must be 30 characters long")
		}
		return nil
	}

	pinEntry := widget.NewEntry()
	pinEntry.SetText(tf.th.PIN())
	pinEntry.Validator = func(s string) error {
		if len(s) != 4 {
			return errors.New("PIN must be 4 characters long")
		}
		return nil
	}

	pskEntry := widget.NewEntry()
	pskEntry.SetText(tf.th.PSK())
	pskEntry.Validator = func(s string) error {
		if len(s) != 12 {
			return errors.New("PSK must be 12 characters long")
		}
		return nil
	}

	iskEntry := widget.NewEntry()
	iskEntry.SetText(tf.th.ISK())
	iskEntry.Validator = func(s string) error {
		if len(s) != 12 {
			return errors.New("PIN must be 12 characters long")
		}
		return nil
	}

	form := widget.NewForm(
		widget.NewFormItem("VIN:", vinEntry),
		widget.NewFormItem("ECU Desc:", ecuEntry),
		widget.NewFormItem("Part No:", partEntry),
		widget.NewFormItem("Soft Ver:", softEntry),
		widget.NewFormItem("PIN:", pinEntry),
		widget.NewFormItem("PSK (Hex):", pskEntry),
		widget.NewFormItem("ISK (Hex):", iskEntry),
	)

	d := dialog.NewCustomConfirm(
		"Edit T8 Firmware Info",
		"Save changes",
		"Cancel",
		container.NewVScroll(form),
		func(save bool) {
			if save {
				if len(vinEntry.Text) == 17 {
					tf.th.SetVIN(vinEntry.Text)
				}
				if len(ecuEntry.Text) == 16 {
					tf.th.SetEcuDescription(ecuEntry.Text)
				}
				if len(partEntry.Text) == 8 {
					tf.th.SetPartNumber(partEntry.Text)
				}
				if len(softEntry.Text) == 30 {
					tf.th.SetSoftwareVersion(softEntry.Text)
				}
				if len(pinEntry.Text) == 4 {
					tf.th.SetPIN(pinEntry.Text)
				}
				if len(pskEntry.Text) == 12 {
					tf.th.SetPSK(pskEntry.Text)
				}
				if len(iskEntry.Text) == 12 {
					tf.th.SetISK(iskEntry.Text)
				}
				done := make(chan bool)
				d := dialog.NewConfirm("Confirmation", "Be careful, your car will not start "+
					"if you change immo data, are you sure to continue ??", func(b bool) {
					done <- b
				}, fyne.CurrentApp().Driver().AllWindows()[0])
				d.Show()

				go func() {
					result := <-done
					if result {
						tf.SaveFlashBlock()
						dialog.ShowInformation("Success", "Data has been stored in T8 bin file.", win)
					}
				}()

			}
		},
		win,
	)

	d.Resize(fyne.NewSize(450, 500))
	d.Show()
}
