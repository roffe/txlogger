package datalogger

import (
	"errors"
	"slices"
	"testing"
	"time"

	symbol "github.com/roffe/ecusymbol"
)

// sysvarFormat prints whole numbers with no decimals, the external wideband
// lambda with three and every other sysvar with two.
func TestSysvarFormat(t *testing.T) {
	tests := []struct {
		name string
		v    float64
		want string
	}{
		{"Rpm", 3000, "3000"},
		{"Rpm", -12, "-12"},
		{"Rpm", 0, "0"},
		{"Boost", 1.234, "1.23"},
		{"Boost", 0.005, "0.01"},
		{EXTERNALWBLSYM, 0.9876, "0.988"},
		{EXTERNALWBLSYM, 1, "1"},
		{LAMBDAADSCANNER, 0.9876, "0.99"},
	}
	for _, tt := range tests {
		if got := string(sysvarFormat(tt.name)(nil, tt.v)); got != tt.want {
			t.Errorf("sysvarFormat(%q)(%v) = %q, want %q", tt.name, tt.v, got, tt.want)
		}
	}
}

// symbolFormat derives the decimals from the correction factor, matching
// symbol.StringValue for every factor it knows.
func TestSymbolFormatMatchesStringValue(t *testing.T) {
	tests := []struct {
		factor float64
		want   string // formatting of raw 1234 * factor
	}{
		{1, "1234"},
		{0.1, "123.4"},
		{0.01, "12.34"},
		{0.0078125, "9.64"},
		{0.0009765625, "1.21"},
		{0.00390625, "4.82"},
		{0.004, "4.94"},
		{0.001, "1.234"},
		{0.5, "617"}, // unknown factor: no decimals
	}
	for _, tt := range tests {
		sym := &symbol.Symbol{Name: "X", Length: 2, Correctionfactor: tt.factor}
		if err := sym.SetData([]byte{0x04, 0xD2}); err != nil { // 1234
			t.Fatal(err)
		}
		got := string(symbolFormat(tt.factor)(nil, sym.Float64()))
		if got != tt.want {
			t.Errorf("factor %v: got %q, want %q", tt.factor, got, tt.want)
		}
		if sv := sym.StringValue(); got != sv {
			t.Errorf("factor %v: symbolFormat %q differs from StringValue %q", tt.factor, got, sv)
		}
	}
}

// Channels read their source on every call, so a sysvar or symbol channel
// built once keeps following the live value; Append extends the buffer it is
// given.
func TestChannelsFollowTheirSource(t *testing.T) {
	sv := NewThreadSafeMap()
	sysCh := newSysvarChannel(sv, "In.v_Vehicle")
	if sysCh.Name != "In.v_Vehicle" || sysCh.Value() != 0 {
		t.Fatalf("unset sysvar channel = %q %v", sysCh.Name, sysCh.Value())
	}
	sv.Set("In.v_Vehicle", 87.25)
	if sysCh.Value() != 87.25 || sysCh.String() != "87.25" {
		t.Fatalf("sysvar channel = %v %q", sysCh.Value(), sysCh.String())
	}

	sym := &symbol.Symbol{Name: "MAF.m_AirInlet", Length: 2, Correctionfactor: 0.1}
	symCh := newSymbolChannel(sym)
	_ = sym.SetData([]byte{0x01, 0x00})
	if symCh.Name != "MAF.m_AirInlet" || symCh.String() != "25.6" {
		t.Fatalf("symbol channel = %q %q", symCh.Name, symCh.String())
	}
	_ = sym.SetData([]byte{0x00, 0x05})
	if got := string(symCh.Append([]byte("x="))); got != "x=0.5" {
		t.Fatalf("Append = %q", got)
	}

	v := 1.0
	fnCh := newFunctionChannel("F", func() float64 { return v })
	v = 2.0 / 3
	if fnCh.String() != "0.67" || fnCh.Value() != 2.0/3 {
		t.Fatalf("function channel = %v %q", fnCh.Value(), fnCh.String())
	}
}

// ThreadSafeMap reports zero for missing names and Exists/Keys/Delete track
// what has been set.
func TestThreadSafeMap(t *testing.T) {
	m := NewThreadSafeMap()
	if m.Exists("a") || m.Get("a") != 0 || len(m.Keys()) != 0 {
		t.Fatal("new map not empty")
	}
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("a", 3)
	keys := m.Keys()
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"a", "b"}) || m.Get("a") != 3 {
		t.Fatalf("keys %v a=%v", keys, m.Get("a"))
	}
	m.Delete("a")
	if m.Exists("a") || !m.Exists("b") || m.Get("a") != 0 {
		t.Fatal("Delete did not remove only a")
	}
	m.Delete("missing") // no-op
}

// Read and write requests start with the full length outstanding and report
// the result passed to Complete through Wait.
func TestDataRequest(t *testing.T) {
	r := NewReadDataRequest(0xF00100, 12)
	if r.Address != 0xF00100 || r.Length != 12 || r.Left != 12 || r.Len() != 12 || r.Data != nil {
		t.Fatalf("read request %+v", r)
	}
	if r.String() != "00F00100: 12" {
		t.Fatalf("String = %q", r.String())
	}
	w := NewWriteDataRequest(0x1000, []byte{1, 2, 3})
	if w.Length != 3 || w.Left != 3 || !slices.Equal(w.Data, []byte{1, 2, 3}) {
		t.Fatalf("write request %+v", w)
	}

	go r.Complete(nil)
	if err := r.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	boom := errors.New("boom")
	w.Complete(boom)
	if err := w.Wait(); !errors.Is(err, boom) {
		t.Fatalf("Wait = %v, want boom", err)
	}
}

// Wait on a request that was not built by a constructor fails instead of
// blocking forever.
func TestDataRequestWaitWithoutChannel(t *testing.T) {
	if err := (&DataRequest{}).Wait(); err == nil {
		t.Fatal("Wait on zero request succeeded")
	}
}

// Completing a request whose result slot is already taken keeps the first
// result rather than blocking the logger loop.
func TestDataRequestCompleteKeepsFirstResult(t *testing.T) {
	r := NewReadDataRequest(0, 1)
	first := errors.New("first")
	r.respChan <- first
	r.Complete(errors.New("second"))
	if err := r.Wait(); !errors.Is(err, first) {
		t.Fatalf("Wait = %v, want first", err)
	}
}

// requeue hands a partly served request back to the loop, and fails it when
// another request already holds the slot.
func TestRequeue(t *testing.T) {
	ch := make(chan *DataRequest, 1)
	a := NewReadDataRequest(0, 1)
	requeue(ch, a)
	if got := <-ch; got != a {
		t.Fatal("request not requeued")
	}

	ch <- NewReadDataRequest(1, 1)
	b := NewReadDataRequest(2, 1)
	requeue(ch, b)
	done := make(chan error, 1)
	go func() { done <- b.Wait() }()
	select {
	case err := <-done:
		if err == nil || err.Error() != "request queue full" {
			t.Fatalf("Wait = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request was not failed")
	}
}
