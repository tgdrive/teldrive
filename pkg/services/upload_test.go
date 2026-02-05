package services

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestMsgDocument_NilPointerScenarios(t *testing.T) {
	tests := []struct {
		name    string
		input   *tg.Message
		wantNil bool
		wantOk  bool
	}{
		{
			name:    "nil message",
			input:   nil,
			wantNil: true,
			wantOk:  false,
		},
		{
			name:    "empty message with nil inner fields",
			input:   &tg.Message{ID: 0},
			wantNil: true,
			wantOk:  false,
		},
		{
			name: "message with nil Media",
			input: &tg.Message{
				ID:    1,
				Media: nil,
			},
			wantNil: true,
			wantOk:  false,
		},
		{
			name: "message with non-document media",
			input: &tg.Message{
				ID: 1,
				Media: &tg.MessageMediaPhoto{
					Photo: &tg.Photo{},
				},
			},
			wantNil: true,
			wantOk:  false,
		},
		{
			name: "message with MessageMediaDocument but nil Document",
			input: &tg.Message{
				ID: 1,
				Media: &tg.MessageMediaDocument{
					Document: nil,
				},
			},
			wantNil: true,
			wantOk:  false,
		},
		{
			name: "message with MessageMediaDocument containing empty Document (size 0)",
			input: &tg.Message{
				ID: 1,
				Media: &tg.MessageMediaDocument{
					Document: &tg.Document{Size: 0}, // Empty Document - this is valid but has size 0
				},
			},
			wantNil: false, // Returns a doc (with size 0), caller handles validation
			wantOk:  true,
		},
		{
			name: "valid document message",
			input: &tg.Message{
				ID: 1,
				Media: &tg.MessageMediaDocument{
					Document: &tg.Document{
						ID:   123,
						Size: 1024,
					},
				},
			},
			wantNil: false,
			wantOk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should NOT panic for any input
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("msgDocument panicked on input %+v: %v", tt.input, r)
				}
			}()

			doc, ok := msgDocument(tt.input)

			if tt.wantNil {
				if doc != nil {
					t.Errorf("msgDocument() = %v, want nil", doc)
				}
			}
			if ok != tt.wantOk {
				t.Errorf("msgDocument() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func TestMsgDocument_AsNotEmptyNilCheck(t *testing.T) {
	// Test case where AsNotEmpty might return nil with ok=true
	t.Run("message with zero ID triggers AsNotEmpty false", func(t *testing.T) {
		msg := &tg.Message{ID: 0}
		doc, ok := msgDocument(msg)
		if doc != nil || ok {
			t.Error("Expected nil, false for message with zero ID")
		}
	})
}
