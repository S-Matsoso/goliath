package grants

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestParseBatchCSVEnforcesHeaderAndRowLimit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantLen int
	}{
		{
			name:    "valid rows",
			input:   "national_id,last_name\nid-one,Name\nid-two,Other\n",
			wantLen: 2,
		},
		{
			name:    "invalid header",
			input:   "id,last_name\nid-one,Name\n",
			wantErr: true,
		},
		{
			name:    "empty data",
			input:   "national_id,last_name\n",
			wantErr: true,
		},
		{
			name:    "extra column",
			input:   "national_id,last_name\nid-one,Name,unexpected\n",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows, err := parseBatchCSV(strings.NewReader(test.input))
			if (err != nil) != test.wantErr {
				t.Fatalf("parseBatchCSV() error = %v, wantErr %v", err, test.wantErr)
			}
			if len(rows) != test.wantLen {
				t.Fatalf("parseBatchCSV() returned %d rows, want %d", len(rows), test.wantLen)
			}
		})
	}

	var input strings.Builder
	input.WriteString("national_id,last_name\n")
	for row := 0; row < 5001; row++ {
		fmt.Fprintf(&input, "id-%d,Name\n", row)
	}
	if _, err := parseBatchCSV(strings.NewReader(input.String())); err == nil {
		t.Fatal("parseBatchCSV() accepted more than 5,000 rows")
	}
}

func TestWriteBatchCSVExcludesNationalIDsAndHidesMismatches(t *testing.T) {
	var output bytes.Buffer
	err := writeBatchCSV(&output, []batchResult{
		{Match: true, Alive: true, DateOfBirth: "2000-01-02", District: "District A"},
		{},
	})
	if err != nil {
		t.Fatalf("writeBatchCSV() error = %v", err)
	}
	if strings.Contains(output.String(), "national_id") || strings.Contains(output.String(), "id-one") {
		t.Fatalf("batch output contains an input identifier: %q", output.String())
	}

	reader := csv.NewReader(&output)
	header, err := reader.Read()
	if err != nil {
		t.Fatalf("read CSV header: %v", err)
	}
	wantHeader := []string{"row_number", "match", "is_alive", "date_of_birth", "district"}
	if strings.Join(header, ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("CSV header = %v, want %v", header, wantHeader)
	}
	match, err := reader.Read()
	if err != nil {
		t.Fatalf("read matching result: %v", err)
	}
	if strings.Join(match, ",") != "1,true,true,2000-01-02,District A" {
		t.Fatalf("matching result = %v", match)
	}
	mismatch, err := reader.Read()
	if err != nil {
		t.Fatalf("read mismatch result: %v", err)
	}
	if strings.Join(mismatch, ",") != "2,false,,," {
		t.Fatalf("mismatch result = %v", mismatch)
	}
	if _, err := reader.Read(); err != io.EOF {
		t.Fatalf("unexpected extra result: %v", err)
	}
}
