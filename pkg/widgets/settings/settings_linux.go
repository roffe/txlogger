package settings

import (
	sdialog "github.com/sqweek/dialog"
)

func selectFolder() (string, error) {
	return sdialog.Directory().Title("Select log folder").Browse()
}
