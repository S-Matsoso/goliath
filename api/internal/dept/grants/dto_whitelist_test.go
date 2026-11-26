package grants

import (
	"testing"

	"github.com/S-Matsoso/goliath/internal/testutil"
)

func TestGrantsResponseDTOWhitelist(t *testing.T) {
	district := "Maseru"
	testutil.AssertJSONWhitelists(t, []testutil.JSONWhitelistCase{
		{
			Name: "death alert",
			Value: deathAlertDTO{
				DateOfDeath: "2000-01-01", District: &district,
			},
			Allowed: []string{"date_of_death", "district"},
		},
	})
}
