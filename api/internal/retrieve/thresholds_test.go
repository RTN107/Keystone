package retrieve

import (
	"context"
	"testing"
)

// The shipped thresholds are pinned against scores measured on the demo documents with real
// embeddings (voyage-4-lite). Changing a default must be a deliberate edit here, and it must
// keep clear of every demo score. Your own deployment can override both through
// KEYSTONE_RELEVANCE_FLOOR and KEYSTONE_BLOCK_MARGIN without touching this test.
func TestShippedThresholdsKeepClearOfTheDemoScores(t *testing.T) {
	th := DefaultThresholds
	if th.Floor != 0.40 || th.Margin != 0.05 {
		t.Fatalf("thresholds changed to %+v: re-run TestLiveCalibration and update this test", th)
	}
	const clear = 0.05 // well beyond the 0.001 run-to-run drift, and the 0.01 rule
	floorBounds := []struct {
		name  string
		score float64
		below bool    // the floor must be above this score
		room  float64 // required distance
	}{
		{"Q5 wifi, strongest demo non-match", 0.2978, true, clear},
		{"Q2 production freeze, weakest demo match", 0.4774, false, clear},
		// Not demo questions, and the thinnest margins the floor has: an unrelated
		// question that happens to sit near the corpus, and a weakly phrased true
		// match. Both are known limits of a similarity floor (see Limits in the README).
		{"weather in Bengaluru, strongest unrelated non-match", 0.3643, true, 0.03},
		{"client system down, highest true match under the floor", 0.3748, false, -0.03},
	}
	for _, b := range floorBounds {
		if d := th.Floor - b.score; (b.below && d < b.room) || (!b.below && -d < b.room) {
			t.Errorf("floor %.2f is within %.2f of %s (%.4f)", th.Floor, b.room, b.name, b.score)
		}
	}
	// Margins: the smallest blocked demo margin is +0.267 (Q4 asked by Delivery),
	// the smallest non-demo blocked margin is +0.129, and the largest margin of a
	// question that must not be blocked is -0.129.
	if th.Margin > 0.267-clear || th.Margin < -0.129+clear {
		t.Errorf("margin %.2f is too close to a demo margin", th.Margin)
	}
}

func TestPassagesUnderTheFloorAreDroppedWhenOthersClearIt(t *testing.T) {
	w := newWorld(t)
	w.seed(t)
	// A second readable document that shares only a couple of words with the question.
	w.ingest(t, "handover", "Handover Notes", "## Handover\n\nWrite a handover note covering access and open tickets for the next engineer.", w.delivery)
	q := "priority ticket breaches response target page delivery lead incident bridge"

	// Precondition: the search really does return a readable passage under the floor,
	// otherwise this test could not tell a filtered result from an unfiltered one.
	raw, err := w.ret.search(context.Background(), w.delivery, vecLit(q))
	if err != nil {
		t.Fatal(err)
	}
	var weak int
	for _, p := range raw {
		if p.Score() < testThresholds.Floor {
			weak++
		}
	}
	if weak == 0 || len(raw) < 2 {
		t.Fatalf("the fixture has no weak readable passage (%d of %d)", weak, len(raw))
	}

	res, err := w.ret.Retrieve(context.Background(), w.delivery, q)
	if err != nil || res.Outcome != Answered {
		t.Fatalf("%v %s", err, res.Describe())
	}
	if len(res.Passages) != 1 {
		t.Errorf("only the passage over the floor should be used, got %d", len(res.Passages))
	}
	for _, p := range res.Passages {
		if p.Score() < testThresholds.Floor {
			t.Errorf("a passage scoring %.3f is under the floor %.2f", p.Score(), testThresholds.Floor)
		}
	}
}
