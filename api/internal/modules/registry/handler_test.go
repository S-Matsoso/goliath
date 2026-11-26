package registry

import (
	"testing"
	"time"

	"github.com/S-Matsoso/goliath/internal/db"
)

func TestPoliceVerdictDoesNotRevealStatusOnMismatch(t *testing.T) {
	person := db.Person{
		DateOfBirth: time.Date(1990, time.January, 1, 0, 0, 0, 0, time.FixedZone("local", 2*60*60)),
	}

	got := policeVerdict(person, time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC))
	if !got.Match || !got.IsAlive {
		t.Fatalf("same calendar date verdict = %+v, want matched and alive", got)
	}

	got = policeVerdict(person, time.Date(1991, time.January, 1, 0, 0, 0, 0, time.UTC))
	if got.Match || got.IsAlive {
		t.Fatalf("mismatch verdict = %+v, want no match and no alive status", got)
	}

	deceasedAt := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	person.DeceasedAt = &deceasedAt
	got = policeVerdict(person, time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC))
	if !got.Match || got.IsAlive {
		t.Fatalf("deceased verdict = %+v, want matched and not alive", got)
	}
}
