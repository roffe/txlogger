package datalogger

import (
	"bytes"
	"testing"
)

// The ECU reads the address from dataBuf[4:7] and the length from dataBuf[3],
// see case APPT_DBMA in SetUpDynamicallyDefinedRegister (APP_Apptool.c).
func TestDBMAPayload(t *testing.T) {
	got := dbmaPayload(0x123456, 4)
	want := []byte{0xF0, APPT_DBMA, 0x00, 0x04, 0x12, 0x34, 0x56}
	if !bytes.Equal(got, want) {
		t.Errorf("got % X, want % X", got, want)
	}
}
