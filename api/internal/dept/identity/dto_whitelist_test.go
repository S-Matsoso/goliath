package identity

import (
	"testing"

	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/testutil"
)

func TestIdentityResponseDTOWhitelists(t *testing.T) {
	value := "value"
	testutil.AssertJSONWhitelists(t, []testutil.JSONWhitelistCase{
		{
			Name: "person record",
			Value: personDTO{
				ID: 1, NationalIDMasked: "*********1234", FirstNames: "A", LastName: "B",
				DateOfBirth: "2000-01-01", Sex: db.SexFemale, CitizenshipStatus: db.CitizenshipCitizen,
				MaritalStatus: db.MaritalSingle, DeceasedAt: &value, IDBlocked: true, IDBlockReason: &value,
				Addresses:  []addressDTO{{Line1: "A", Suburb: &value, City: "B", Province: "C", District: "D", WardCode: &value, PostalCode: &value, ValidFrom: "2000-01-01", ValidTo: &value, IsCurrent: true}},
				Marriages:  []marriageDTO{{ID: 1, SpouseAID: 1, SpouseBID: 2, MarriedOn: "2000-01-01", DissolvedOn: &value, CertificateNo: "C1"}},
				Biometrics: []biometricDTO{{Kind: "fingerprint", CapturedAt: "2000-01-01T00:00:00Z"}},
			},
			Allowed: []string{"id", "national_id_masked", "first_names", "last_name", "date_of_birth", "sex", "citizenship_status", "marital_status", "deceased_at", "id_blocked", "id_block_reason", "addresses", "marriages", "biometrics"},
		},
		{
			Name: "search result",
			Value: searchResultDTO{
				ID: 1, FirstNames: "A", LastName: "B", DateOfBirth: "2000-01-01",
				NationalIDMasked: "*********1234",
			},
			Allowed: []string{"id", "first_names", "last_name", "date_of_birth", "national_id_masked"},
		},
		{
			Name:    "reveal result",
			Value:   revealDTO{NationalID: "1234567890123"},
			Allowed: []string{"national_id"},
		},
		{
			Name: "change request",
			Value: requestDTO{
				ID: 1, Type: "ID_LOST_STOLEN", Payload: map[string]string{"case_reference": "C1"},
				Department: "police", OfficialID: 2, Status: "PENDING", ReviewedBy: nil,
				ReviewedAt: &value, Reason: &value, CreatedAt: "2000-01-01T00:00:00Z",
			},
			Allowed: []string{"id", "type", "payload", "submitted_by_department", "submitted_by_official", "status", "reviewed_by", "reviewed_at", "reason", "created_at"},
		},
	})
}
