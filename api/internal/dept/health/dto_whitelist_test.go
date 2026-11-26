package health

import (
	"testing"

	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/testutil"
)

func TestHealthResponseDTOWhitelists(t *testing.T) {
	deceased := true
	place, facility, cause := "Maseru", "HOSP-1", "C01"
	testutil.AssertJSONWhitelists(t, []testutil.JSONWhitelistCase{
		{
			Name: "person lookup",
			Value: personDTO{
				Match: true, FirstNames: "A", LastName: "B", DateOfBirth: "2000-01-01",
				Sex: db.SexFemale, IsDeceased: &deceased,
			},
			Allowed: []string{"match", "first_names", "last_name", "date_of_birth", "sex", "is_deceased"},
		},
		{
			Name: "submission",
			Value: submissionDTO{
				ID: 1, Certificate: "C1", Date: "2000-01-01", FirstNames: "A", LastName: "B",
				Place: &place, Facility: &facility, Cause: &cause, RegisteredAt: "2000-01-01T00:00:00Z",
			},
			Allowed: []string{"id", "certificate_no", "date", "first_names", "last_name", "place", "facility_code", "cause_code", "registered_at"},
		},
	})
}
