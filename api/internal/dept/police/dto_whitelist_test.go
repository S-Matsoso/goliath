package police

import (
	"testing"

	"github.com/S-Matsoso/goliath/internal/db"
	"github.com/S-Matsoso/goliath/internal/testutil"
)

func TestPoliceResponseDTOWhitelists(t *testing.T) {
	reviewedAt, reason := "2000-01-02T00:00:00Z", "Reviewed"
	testutil.AssertJSONWhitelists(t, []testutil.JSONWhitelistCase{
		{
			Name: "change request status",
			Value: requestDTO{
				ID: 1, Type: "ID_LOST_STOLEN", Status: "PENDING",
				ReviewedAt: &reviewedAt, Reason: &reason, CreatedAt: "2000-01-01T00:00:00Z",
			},
			Allowed: []string{"id", "type", "status", "reviewed_at", "reason", "created_at"},
		},
		{
			Name: "biometric candidate",
			Value: biometricResultDTO{
				FirstNames: "A", LastName: "B", DateOfBirth: "2000-01-01",
				Sex: db.SexFemale, Alive: true, Score: 0.9,
			},
			Allowed: []string{"first_names", "last_name", "date_of_birth", "sex", "is_alive", "score"},
		},
	})
}
