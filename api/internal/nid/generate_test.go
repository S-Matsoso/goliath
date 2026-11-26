package nid

import (
	"strconv"
	"testing"
	"time"
)

func TestGenerateCreatesValidSexBandedNationalID(t *testing.T) {
	dob := time.Date(1992, time.June, 17, 0, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		sex      string
		citizens string
		parity   int
	}{
		{sex: "M", citizens: "CITIZEN", parity: 1},
		{sex: "F", citizens: "FOREIGN", parity: 0},
		{sex: "X", citizens: "STATELESS", parity: 1},
	} {
		value, err := Generate(dob, test.sex, test.citizens)
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(value, dob); err != nil {
			t.Fatalf("generated ID %q is invalid: %v", value, err)
		}
		sequence, err := strconv.Atoi(value[6:10])
		if err != nil || sequence%2 != test.parity {
			t.Fatalf("sequence band = %q, want parity %d", value[6:10], test.parity)
		}
	}
}

func TestGenerateRejectsUnsupportedFields(t *testing.T) {
	dob := time.Date(1992, time.June, 17, 0, 0, 0, 0, time.UTC)
	if _, err := Generate(dob, "?", "CITIZEN"); err == nil {
		t.Fatal("unsupported sex accepted")
	}
	if _, err := Generate(dob, "M", "UNKNOWN"); err == nil {
		t.Fatal("unsupported citizenship status accepted")
	}
}
