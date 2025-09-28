package ebus_test

import (
	"testing"

	"github.com/roffe/txlogger/pkg/ebus"
)

func TestPublish(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		topic   string
		data    float64
		wantErr bool
	}{
		{
			name:  "test",
			topic: "test",
			data:  1.23,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := ebus.Publish(tt.topic, tt.data)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Publish() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Publish() succeeded unexpectedly")
			}
		})
	}
}

func TestSubscribe(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		topic   string
		wantNil bool
	}{
		{
			name:  "test",
			topic: "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotChan := ebus.Subscribe(tt.topic)
			if gotChan == nil {
				if !tt.wantNil {
					t.Errorf("Subscribe() failed: got nil channel")
				}
				return
			}
			if tt.wantNil {
				t.Fatal("Subscribe() succeeded unexpectedly")
			}
			ebus.Publish(tt.topic, 3.14)
			v := <-gotChan
			if v != 3.14 {
				t.Errorf("Subscribe() got %v, want 3.14", v)
			}
			ebus.Unsubscribe(gotChan)
		})
	}
}

func TestSubscribeFunc(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		topic   string
		wantNil bool
	}{
		{
			name:  "test",
			topic: "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := ebus.SubscribeFunc(tt.topic, func(v float64) {
				if v != 2.71 {
					t.Errorf("SubscribeFunc() got %v, want 2.71", v)
				}
			})
			if cleanup == nil {
				if !tt.wantNil {
					t.Errorf("SubscribeFunc() failed: got nil cleanup function")
				}
				return
			}
			if tt.wantNil {
				t.Fatal("SubscribeFunc() succeeded unexpectedly")
			}
			ebus.Publish(tt.topic, 2.71)
			cleanup()
		})
	}
}
