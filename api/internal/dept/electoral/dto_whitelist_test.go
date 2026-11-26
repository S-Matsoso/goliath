package electoral

import (
	"testing"

	"github.com/S-Matsoso/goliath/internal/testutil"
)

func TestElectoralResponseDTOWhitelists(t *testing.T) {
	ward := "01"
	testutil.AssertJSONWhitelists(t, []testutil.JSONWhitelistCase{
		{
			Name:    "voter roll",
			Value:   voterDTO{FirstNames: "A", LastName: "B", Ward: &ward},
			Allowed: []string{"first_names", "last_name", "ward"},
		},
		{
			Name: "event feed",
			Value: eventDTO{
				EventType: "PERSON_DIED", Ward: &ward, Removed: true,
				CreatedAt: "2000-01-01T00:00:00Z",
			},
			Allowed: []string{"event_type", "ward", "removed", "created_at"},
		},
	})
}
