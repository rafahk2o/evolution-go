package media_test

import (
	"testing"

	"github.com/EvolutionAPI/evolution-go/internal/wacalls/media"
)

func TestPCMInt16LERoundTrip(t *testing.T) {
	got := media.PCMFloat32ToInt16LE([]float32{-1, 0, 1})
	roundTrip := media.PCMInt16LEToFloat32(got)
	if len(roundTrip) != 3 || roundTrip[0] > -0.99 || roundTrip[2] < 0.99 {
		t.Fatalf("unexpected round trip: %v", roundTrip)
	}
}
