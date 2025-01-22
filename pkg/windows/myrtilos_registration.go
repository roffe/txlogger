package windows

import (
	"context"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/roffe/gocan"
	"github.com/roffe/txlogger/pkg/debug"
	"github.com/roffe/txlogger/pkg/kwp2000"
)

var _ fyne.Widget = (*MyrtilosRegistration)(nil)

type MyrtilosRegistration struct {
	mw *MainWindow

	input      *widget.Entry
	text       *widget.Label
	btn        *widget.Button
	output     *widget.List
	outputData binding.StringList

	l binding.DataListener

	widget.BaseWidget
}

func NewMyrtilosRegistration(mw *MainWindow) fyne.Widget {
	mr := &MyrtilosRegistration{
		mw:         mw,
		input:      widget.NewEntry(),
		text:       widget.NewLabel("Enter security key"),
		outputData: binding.NewStringList(),
	}
	mr.ExtendBaseWidget(mr)
	mr.output = widget.NewListWithData(
		mr.outputData,
		func() fyne.CanvasObject {
			return &widget.Label{
				Alignment:  fyne.TextAlignLeading,
				Truncation: fyne.TextTruncateEllipsis,
			}

		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			i := item.(binding.String)
			txt, err := i.Get()
			if err != nil {
				log.Println()
				return
			}
			obj.(*widget.Label).SetText(txt)
		},
	)
	mr.input.Validator = func(s string) error {
		s = strings.ReplaceAll(s, " ", "")
		if len(s) != 8 {
			return errors.New("invalid security key length")
		}
		_, err := hex.DecodeString(s)
		if err != nil {
			return errors.New("invalid security key")
		}
		return nil
	}
	mr.btn = widget.NewButtonWithIcon("Register", theme.InfoIcon(), func() {
		key, err := hex.DecodeString(mr.input.Text)
		if err != nil {
			mr.text.SetText(err.Error())
			return
		}
		if err := mr.register(key); err != nil {
			mr.outputData.Append(err.Error())
		} else {
			mr.outputData.Append("key saved in ecu")
		}
	})
	return mr
}

func (mr *MyrtilosRegistration) CreateRenderer() fyne.WidgetRenderer {
	mr.l = binding.NewDataListener(func() {
		mr.output.ScrollToBottom()
	})
	mr.outputData.AddListener(mr.l)
	return &myrtilosRegistrationRenderer{MyrtilosRegistration: mr}
}

func (mr *MyrtilosRegistration) register(key []byte) error {
	if len(key) != 4 {
		return errors.New("invalid key length")
	}
	if mr.mw.dlc != nil {
		return errors.New("stop logging before registering")
	}

	logFn := func(s string) {
		debug.Do(func() {
			mr.outputData.Append(s)
		})
	}

	adapter, err := mr.mw.settings.CanSettings.GetAdapter("T7", logFn)
	if err != nil {
		return err
	}
	ctx := context.Background()
	c, err := gocan.New(ctx, adapter)
	if err != nil {
		return err
	}
	defer c.Close()
	kwp := kwp2000.New(c)

	log.Println("Starting session")
	if err := kwp.StartSession(ctx, kwp2000.INIT_MSG_ID, kwp2000.INIT_RESP_ID); err != nil {
		return err
	}
	defer func() {
		log.Println("Stopping session")
		kwp.StopSession(ctx)
		time.Sleep(50 * time.Millisecond)
	}()

	gotIt, err := kwp.RequestSecurityAccess(ctx, false)
	if err != nil {
		return err
	}

	if gotIt {
		log.Println("Got security access")
	} else {
		return errors.New("didn't get security access")
	}

	if err := kwp.SendEU0DRegistrationKey(ctx, key); err != nil {
		return err
	}
	return nil
}

var _ fyne.WidgetRenderer = (*myrtilosRegistrationRenderer)(nil)

type myrtilosRegistrationRenderer struct {
	*MyrtilosRegistration
	oldSize fyne.Size
}

func (mr *myrtilosRegistrationRenderer) MinSize() fyne.Size {
	return fyne.NewSize(400, 200)
}

func (mr *myrtilosRegistrationRenderer) Layout(size fyne.Size) {
	if size == mr.oldSize {
		return
	}
	mr.oldSize = size
	mr.text.Resize(fyne.NewSize(size.Width, 30))
	mr.input.Resize(fyne.NewSize(size.Width, 38))
	mr.output.Move(fyne.NewPos(0, 68))
	mr.btn.Resize(fyne.NewSize(size.Width, 30))
	mr.text.Move(fyne.NewPos(0, 0))
	mr.input.Move(fyne.NewPos(0, 30))
	mr.output.Resize(fyne.NewSize(size.Width, size.Height-mr.btn.Size().Height-66))
	mr.btn.Move(fyne.NewPos(0, size.Height-mr.btn.Size().Height))
}

func (mr *myrtilosRegistrationRenderer) Refresh() {
}

func (mr *myrtilosRegistrationRenderer) Destroy() {
	mr.outputData.RemoveListener(mr.l)
}

func (mr *MyrtilosRegistration) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{mr.text, mr.input, mr.output, mr.btn}
}
