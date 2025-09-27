package txconfigurator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/mdns"
	"github.com/roffe/txlogger/pkg/ota"
	"github.com/roffe/txlogger/pkg/txbridge"
)

var _ desktop.Mouseable = (*ConfiguratorWidget)(nil)

type ConfiguratorWidget struct {
	widget.BaseWidget

	client *txbridge.Client

	apSSIDEntry      *widget.Entry
	apPasswordEntry  *widget.Entry
	apChannelSelect  *widget.Select
	wifiModeSelect   *widget.Select
	staSSIDEntry     *widget.SelectEntry
	staPasswordEntry *widget.Entry

	restartButton *widget.Button
	connectButton *widget.Button
	updateButton  *widget.Button
	statusLabel   *widget.Label

	container *fyne.Container
}

func NewConfigurator() *ConfiguratorWidget {
	t := &ConfiguratorWidget{
		client: txbridge.NewClient(),
	}

	t.apChannelSelect = widget.NewSelect([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13"}, func(s string) {
		t.restartButton.Enable()
	})
	t.apChannelSelect.Disable()
	t.apChannelSelect.PlaceHolder = "Channel"

	t.restartButton = widget.NewButtonWithIcon("Save & Restart device", theme.DocumentSaveIcon(), func() {
		t.saveAndRestart()
		t.restartButton.Disable()
	})
	t.restartButton.Disable()

	t.updateButton = widget.NewButtonWithIcon("Update Firmware", theme.UploadIcon(), func() {
		t.updateButton.Disable()
		t.connectButton.Disable()
		go func() {
			defer t.updateButton.Enable()
			defer fyne.Do(func() {
				t.updateButton.Enable()
				t.connectButton.Enable()
			})
			address := "tcp://192.168.4.1:1337"
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if src, err := mdns.Query(ctx, "txbridge.local"); err != nil {
				log.Printf("failed to query mDNS: %v", err)
			} else {
				if src.IsValid() {
					address = fmt.Sprintf("tcp://%s:%d", src.String(), 1337)
				} else {
					log.Printf("No mDNS response, using address: %s", address)
				}
			}

			err := ota.UpdateOTA(ota.Config{
				Port: address,
				Logfunc: func(a ...any) {
					t.statusLabel.SetText(fmt.Sprint(a...))
				},
				ProgressFunc: func(f float64) {
					fyne.Do(func() {
						t.statusLabel.SetText(fmt.Sprintf("Updating firmware: %.1f%%", f))
					})
				},
			})

			if err != nil {
				//dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
				t.statusLabel.SetText("Status: " + err.Error())
				return
			}
		}()
	})

	t.connectButton = widget.NewButtonWithIcon("Connect", theme.MediaPlayIcon(), t.connect)

	t.statusLabel = widget.NewLabel("Status: Disconnected")

	t.apSSIDEntry = widget.NewEntry()
	t.apSSIDEntry.SetPlaceHolder("<Device AP SSID>")
	t.apSSIDEntry.Validator = ssidValidator
	t.apSSIDEntry.OnChanged = func(s string) {
		t.restartButton.Enable()
	}
	t.apSSIDEntry.Disable()

	t.apPasswordEntry = widget.NewEntry()
	t.apPasswordEntry.Validator = passwordValidator
	t.apPasswordEntry.SetPlaceHolder("<Device AP Password>")
	t.apPasswordEntry.OnChanged = func(s string) {
		t.restartButton.Enable()
	}
	t.apPasswordEntry.Disable()

	t.wifiModeSelect = widget.NewSelect([]string{"AP", "STA", "AP+STA"}, func(s string) {
		switch s {
		case "AP":
			t.apSSIDEntry.Enable()
			t.apPasswordEntry.Enable()
			t.wifiModeSelect.Enable()
			t.apChannelSelect.Enable()
			t.staSSIDEntry.Disable()
			t.staPasswordEntry.Disable()
		case "STA":
			t.apSSIDEntry.Disable()
			t.apPasswordEntry.Disable()
			t.wifiModeSelect.Enable()
			t.apChannelSelect.Disable()
			t.staSSIDEntry.Enable()
			t.staPasswordEntry.Enable()
		case "AP+STA":
			t.apSSIDEntry.Enable()
			t.apPasswordEntry.Enable()
			t.wifiModeSelect.Enable()
			t.apChannelSelect.Enable()
			t.staSSIDEntry.Enable()
			t.staPasswordEntry.Enable()
		}
	})
	t.wifiModeSelect.Disable()

	t.staSSIDEntry = widget.NewSelectEntry([]string{})
	t.staSSIDEntry.Validator = ssidValidator
	t.staSSIDEntry.SetPlaceHolder("<STA SSID>")
	t.staSSIDEntry.OnChanged = func(s string) {
		t.restartButton.Enable()
	}
	t.staSSIDEntry.Disable()

	t.staPasswordEntry = widget.NewEntry()
	t.staPasswordEntry.Validator = passwordValidator
	t.staPasswordEntry.SetPlaceHolder("<STA Password>")
	t.staPasswordEntry.OnChanged = func(s string) {
		t.restartButton.Enable()
	}
	t.staPasswordEntry.Disable()

	t.ExtendBaseWidget(t)
	return t.render()
}

func (t *ConfiguratorWidget) connect() {
	err := t.client.Connect()
	if err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	ssidBytes, err := t.getConfig(0x01)
	if err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	t.statusLabel.SetText("Status: Connected")
	t.apSSIDEntry.SetText(string(ssidBytes))

	passwordBytes, err := t.getConfig(0x02)
	if err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	t.apPasswordEntry.SetText(string(passwordBytes))

	wifiModeBytes, err := t.getConfig(0x03)
	if err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	switch wifiModeBytes[0] {
	case 0x00:
		t.wifiModeSelect.SetSelectedIndex(0) // AP mode
	case 0x01:
		t.wifiModeSelect.SetSelectedIndex(1) // STA mode
	case 0x02:
		t.wifiModeSelect.SetSelectedIndex(2) // AP+STA mode
	default:
		dialog.ShowError(errors.New("unknown WiFi mode"), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	staSSIDBytes, err := t.getConfig(0x04)
	if err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	t.staSSIDEntry.SetText(string(staSSIDBytes))
	staPasswordBytes, err := t.getConfig(0x05)
	if err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	t.staPasswordEntry.SetText(string(staPasswordBytes))

	wifiChannel, err := t.getConfig(0x06) // Get WiFi channel
	if err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	t.apChannelSelect.SetSelectedIndex(int(wifiChannel[0]) - 1)

	t.restartButton.Enable()
	t.updateButton.Disable()
	t.connectButton.SetIcon(theme.MediaStopIcon())
	t.connectButton.SetText("Disconnect")
	t.connectButton.OnTapped = t.disconnect
}

func (t *ConfiguratorWidget) disconnect() {
	t.restartButton.Disable()
	t.updateButton.Enable()
	if err := t.client.Disconnect(); err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
	}
	t.connectButton.SetIcon(theme.MediaPlayIcon())
	t.connectButton.SetText("Connect")
	t.connectButton.OnTapped = t.connect
	t.statusLabel.SetText("Status: Disconnected")
}

func ssidValidator(s string) error {
	if len(s) > 63 {
		return errors.New("SSID cannot be longer than 63 characters")
	}
	if len(s) == 0 {
		return errors.New("SSID cannot be empty")
	}
	return nil
}

func passwordValidator(s string) error {
	if len(s) > 63 {
		return errors.New("password cannot be longer than 63 characters")
	}
	if len(s) < 8 {
		return errors.New("password cannot be under 8 characters")
	}
	return nil
}

func (t *ConfiguratorWidget) saveAndRestart() {
	if len(t.apSSIDEntry.Text) == 0 {
		dialog.ShowError(errors.New("AP SSID cannot be empty"), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	if len(t.apPasswordEntry.Text) == 0 {
		dialog.ShowError(errors.New("password cannot be empty"), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	if len(t.apSSIDEntry.Text) > 63 {
		dialog.ShowError(errors.New("AP SSID cannot be longer than 63 characters"), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	if len(t.apPasswordEntry.Text) > 63 {
		dialog.ShowError(errors.New("AP password cannot be longer than 63 characters"), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	if err := t.setConfig(0x01, []byte(t.apSSIDEntry.Text)); err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	if err := t.setConfig(0x02, []byte(t.apPasswordEntry.Text)); err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	if err := t.setConfig(0x03, []byte{byte(t.wifiModeSelect.SelectedIndex())}); err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	if t.wifiModeSelect.SelectedIndex() == 1 || t.wifiModeSelect.SelectedIndex() == 2 {
		if err := t.setConfig(0x04, []byte(t.staSSIDEntry.Text)); err != nil {
			dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
			return
		}
		if err := t.setConfig(0x05, []byte(t.staPasswordEntry.Text)); err != nil {
			dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
			return
		}
	}

	if err := t.setConfig(0x06, []byte{byte(t.apChannelSelect.SelectedIndex() + 1)}); err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	if err := t.client.SendCommand('q', nil); err != nil {
		dialog.ShowError(err, fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	t.restartButton.Disable()
	t.statusLabel.SetText("Status: Settings saved, device will restart")
	time.Sleep(100 * time.Millisecond)
	t.disconnect()
}

func (t *ConfiguratorWidget) setConfig(opt byte, value []byte) error {
	if len(value) == 0 {
		value = []byte{0x00} // Ensure we send at least one byte
	}
	payload := append([]byte{opt}, value...)
	if err := t.client.SendCommand('C', payload); err != nil {
		return fmt.Errorf("failed to set config option %d: %w", opt, err)
	}
	return nil
}

func (t *ConfiguratorWidget) getConfig(opt byte) ([]byte, error) {
	if err := t.client.SendCommand('G', []byte{opt}); err != nil {
		return nil, err
	}
	resp, err := t.client.ReadCommand(2 * time.Second)
	if err != nil {
		return nil, err
	}
	return resp.Data[1:], nil
}

func (t *ConfiguratorWidget) render() *ConfiguratorWidget {
	t.container = container.NewStack(
		container.NewBorder(
			nil,
			container.NewBorder(
				nil,
				t.statusLabel,
				nil,
				nil,
				container.NewGridWithColumns(2,
					t.connectButton,
					t.updateButton,
				),
			),
			nil,
			nil,
			container.NewBorder(
				nil,
				t.restartButton,
				container.NewVBox(
					widget.NewLabel("WiFi Mode:"),
					widget.NewLabel("AP SSID:"),
					widget.NewLabel("AP Channel:"),
					widget.NewLabel("AP Password:"),
					widget.NewLabel("STA SSID:"),
					widget.NewLabel("STA Password:"),
				),
				nil,
				container.NewVBox(
					t.wifiModeSelect,
					t.apSSIDEntry,
					t.apChannelSelect,
					t.apPasswordEntry,
					t.staSSIDEntry,
					t.staPasswordEntry,
				),
			),
		),
	)
	return t
}

func (t *ConfiguratorWidget) CreateRenderer() fyne.WidgetRenderer {
	return &ConfiguratorWidgetRenderer{
		t: t,
	}
}

func (t *ConfiguratorWidget) MouseDown(e *desktop.MouseEvent) {
}

func (t *ConfiguratorWidget) MouseUp(e *desktop.MouseEvent) {
}

type ConfiguratorWidgetRenderer struct {
	t *ConfiguratorWidget
}

func (tr *ConfiguratorWidgetRenderer) Layout(space fyne.Size) {
	tr.t.container.Resize(space)
	// do stuff
}

func (tr *ConfiguratorWidgetRenderer) MinSize() fyne.Size {
	return tr.t.container.MinSize()
}

func (tr *ConfiguratorWidgetRenderer) Refresh() {

}

func (tr *ConfiguratorWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{tr.t.container}
}

func (tr *ConfiguratorWidgetRenderer) Destroy() {
}
