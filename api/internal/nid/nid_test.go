package nid

import (
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	dob := time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		id   string
		dob  time.Time
		want bool
	}{
		{name: "valid", id: "9001011234001", dob: dob, want: true},
		{name: "wrong length", id: "900101123400", dob: dob},
		{name: "non-digit", id: "90010112340a1", dob: dob},
		{name: "birth date mismatch", id: "9001011234001", dob: dob.AddDate(1, 0, 0)},
		{name: "invalid check digit", id: "9001011234002", dob: dob},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.id, tt.dob)
			if (err == nil) != tt.want {
				t.Fatalf("Validate() error = %v, want valid = %t", err, tt.want)
			}
		})
	}
}

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid", id: "9001011234001", want: true},
		{name: "wrong length", id: "900101123400"},
		{name: "non-digit", id: "90010112340a1"},
		{name: "invalid check digit", id: "9001011234002"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFormat(tt.id)
			if (err == nil) != tt.want {
				t.Fatalf("ValidateFormat() error = %v, want valid = %t", err, tt.want)
			}
		})
	}
}
