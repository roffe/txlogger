package txweb

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/txlogger/pkg/txwebclient"
	"github.com/roffe/txlogger/pkg/widgets/progressmodal"
)

const (
	Endpoint = "https://txweb.roffe.nu"
)

type Widget struct {
	widget.BaseWidget

	container *fyne.Container

	client *txwebclient.Client

	files    []txwebclient.File
	fileList *widget.List

	loginContainer    *fyne.Container
	filelistContainer *fyne.Container

	LoadFileFunc func(name string, data []byte) error
	CloseFunc    func()
}

func New() *Widget {
	app := fyne.CurrentApp()
	t := &Widget{
		client: txwebclient.New(
			txwebclient.WithEndpoint(Endpoint),
			txwebclient.WithToken(app.Preferences().String("txweb_token")),
		),
		LoadFileFunc: func(name string, data []byte) error { return nil },
		CloseFunc:    func() {},
	}
	t.ExtendBaseWidget(t)

	if expires := app.Preferences().Int("txweb_token_expires"); expires > 0 {
		if time.Now().Unix() > int64(expires) {
			app.Preferences().RemoveValue("txweb_token")
			app.Preferences().RemoveValue("txweb_token_expires")
		}
	}

	t.loginContainer = t.loginForm()
	t.filelistContainer = t.createFileList()

	if app.Preferences().String("txweb_token") == "" {
		t.loginContainer.Show()
		t.filelistContainer.Hide()
	} else {
		t.loginContainer.Hide()
		t.filelistContainer.Show()
	}

	t.container = container.NewStack(
		t.loginContainer,
		t.filelistContainer,
	)

	return t
}

func (t *Widget) UploadFile(filename string) error {
	return t.client.UploadFile(filename)
}

func (t *Widget) createFileList() *fyne.Container {
	t.fileList = widget.NewList(
		func() int {
			return len(t.files)
		},
		func() fyne.CanvasObject {
			return t.NewFileEntry()
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < 0 || i >= len(t.files) {
				return
			}
			f := t.files[i]
			o.(*FileEntry).Set(i, f)

		},
	)

	return container.NewBorder(
		widget.NewButtonWithIcon("Logout", theme.LogoutIcon(), func() {
			app := fyne.CurrentApp()
			app.Preferences().RemoveValue("txweb_token")
			app.Preferences().RemoveValue("txweb_token_expires")
			t.client.SetToken("")
			t.loginContainer.Show()
			t.filelistContainer.Hide()
		}),
		widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
			var err error
			t.files, err = t.client.ListFiles()
			if err != nil {
				t.Error(err)
				return
			}
			t.fileList.Refresh()
		}),
		nil,
		nil,
		t.fileList,
	)
}

func (t *Widget) loginForm() *fyne.Container {
	usernameEntry := widget.NewEntry()
	passwordEntry := widget.NewPasswordEntry()

	form := widget.NewForm(
		widget.NewFormItem("Username", usernameEntry),
		widget.NewFormItem("Password", passwordEntry),
	)
	form.SubmitText = "Login"

	form.OnSubmit = func() {
		if usernameEntry.Text == "" || passwordEntry.Text == "" {
			t.Error(errors.New("please fill in all fields"))
			return
		}

		token, exp, err := txwebclient.Login(Endpoint, usernameEntry.Text, passwordEntry.Text)
		if err != nil {
			t.Error(err)
			return
		}

		app := fyne.CurrentApp()
		app.Preferences().SetString("txweb_token", token)
		app.Preferences().SetInt("txweb_token_expires", int(exp))

		t.client.SetToken(token)

		t.refreshFiles()

		t.loginContainer.Hide()
		t.filelistContainer.Show()

		passwordEntry.SetText("")
	}

	return container.NewGridWithRows(3,
		container.NewStack(),
		form,
		container.NewStack(),
	)
}

func (t *Widget) refreshFiles() {
	var err error
	if t.files, err = t.client.ListFiles(); err != nil {
		log.Println("ListFiles error:", err)
	}
	t.fileList.Refresh()
}

func (t *Widget) Error(err error) {
	w := fyne.CurrentApp().Driver().AllWindows()
	if len(w) < 1 {
		log.Println("Error:", err)
		return
	}
	errd := dialog.NewError(err, w[0])
	wms := w[0].Canvas().Size()
	wms.Height = wms.Height / 8
	wms.Width = wms.Width / 2
	errd.Resize(wms)
	errd.Show()
}

func (t *Widget) CreateRenderer() fyne.WidgetRenderer {
	if t.filelistContainer.Visible() {
		t.refreshFiles()
	}
	return widget.NewSimpleRenderer(t.container)
}

// ------------------------------------------------------------------------------
// FileEntry represents a single file entry in the file list.
type FileEntry struct {
	widget.BaseWidget

	container *fyne.Container

	modtimeLabel  *widget.Label
	sizeLabel     *widget.Label
	filenameLabel *widget.Label

	openButton *widget.Button
	deleteBtn  *widget.Button

	t *Widget
}

func (t *Widget) NewFileEntry() *FileEntry {
	fe := &FileEntry{
		t:             t,
		filenameLabel: widget.NewLabel(""),
		sizeLabel:     widget.NewLabel(""),
		modtimeLabel:  widget.NewLabel(""),
		openButton:    widget.NewButtonWithIcon("", theme.FolderOpenIcon(), nil),
		deleteBtn:     widget.NewButtonWithIcon("", theme.DeleteIcon(), nil),
	}

	fe.filenameLabel.Importance = widget.HighImportance
	fe.filenameLabel.Wrapping = fyne.TextWrapBreak
	fe.filenameLabel.Selectable = true

	fe.container = container.NewBorder(
		nil,
		nil,
		container.NewVBox(
			fe.modtimeLabel,
			fe.sizeLabel,
		),
		container.NewHBox(
			fe.openButton,
			fe.deleteBtn,
		),
		fe.filenameLabel,
	)
	fe.ExtendBaseWidget(fe)
	return fe
}

func (fe *FileEntry) Set(idx int, f txwebclient.File) {
	switch filepath.Ext(f.Name) {
	case ".bin":
		fe.openButton.SetIcon(theme.FileIcon())
	case ".t7l", ".t8l", ".csv":
		fe.openButton.SetIcon(theme.DocumentIcon())
	default:
		fe.openButton.SetIcon(theme.QuestionIcon())
	}

	fe.filenameLabel.SetText(f.Name)
	fe.modtimeLabel.SetText(f.ModTime.Format("2006-01-02 15:04:05"))
	fe.sizeLabel.SetText(fmt.Sprintf("%.2f KB", float64(f.Size)/1024.0))

	fe.openButton.OnTapped = func() {

		fe.openButton.SetIcon(theme.FileIcon())
		p := progressmodal.New(
			fyne.CurrentApp().Driver().CanvasForObject(fe.t.fileList),
			"Downloading "+f.Name,
		)
		sz := fe.t.container.Size()
		sz.Width = sz.Width / 3
		sz.Height = sz.Height / 5
		p.Show()
		p.Resize(sz)
		go func() {
			defer fyne.Do(p.Hide)
			data, err := fe.t.client.DownloadFile(f.Name)
			if err != nil {
				fyne.Do(func() {
					fe.t.Error(err)
				})
				return
			}
			//fe.t.statusLabel.SetText(fmt.Sprintf("Downloaded %s (%d bytes)", f.Name, len(data)))
			fyne.Do(func() {
				if err := fe.t.LoadFileFunc(f.Name, data); err != nil {
					fe.t.Error(err)
					return
				}
				fe.t.CloseFunc()
			})
		}()

	}
	fe.deleteBtn.OnTapped = func() {
		fmt.Println("Delete", f.Name)
		err := fe.t.client.DeleteFile(f.Name)
		if err != nil {
			fe.t.Error(err)
			return
		}
		fe.t.files = append(fe.t.files[:idx], fe.t.files[idx+1:]...)
		fe.t.fileList.Refresh()
	}
}

func (t *FileEntry) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.container)
}

/*

func (t *TxWeb) login(username, password string) error {
	r := strings.NewReader("{\"username\":\"" + username + "\",\"password\":\"" + password + "\"}")
	resp, err := http.Post(Endpoint+"/login", "application/json", r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("login failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var loginResp struct {
		Message string `json:"message"`
		Token   string `json:"token"`
		User    string `json:"user"`
		Expires int64  `json:"expires"`
	}
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return err
	}

	log.Printf("login response: %s, token: %s, user: %s", loginResp.Message, loginResp.Token, loginResp.User)

	t.mw.app.Preferences().SetString("txweb_token", loginResp.Token)
	t.mw.app.Preferences().SetInt("txweb_token_expires", int(loginResp.Expires))

	return nil
}

func (t *TxWeb) listFiles() ([]File, error) {
	token := t.mw.app.Preferences().String("txweb_token")
	if token == "" {
		return nil, errors.New("not logged in")
	}
	req, err := http.NewRequest("GET", Endpoint+"/api/files", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to list files")
	}

	var files []File
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}

	return files, nil
}
*/
