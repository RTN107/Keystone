package retrieve

// Outcome is what Keystone does with a question.
type Outcome string

const (
	Answered Outcome = "answered"
	Blocked  Outcome = "blocked"
	NoMatch  Outcome = "no_match"
)

// NoScore stands for "there is nothing to score": a team with no readable
// document, or a deployment with nothing the team cannot read. Cosine similarity
// never goes below -1, so this is below every real score.
const NoScore = -2.0

// Thresholds are the two numbers the decision uses.
//
// Floor is the relevance floor: a passage scoring below it is not treated as
// relevant, whether it is one the asker may read or one they may not.
//
// Margin is how far the best passage the asker may NOT read must beat the best
// passage they MAY read before the question counts as blocked.
//
// Scores are cosine similarity from the embedding model. The shipped values were measured with
// voyage-4-lite on the three demo documents in demo-docs/ (56 labelled questions, each asked as
// each of the three sample teams). Other documents, a different model or a different
// language will score differently, so treat them as a starting point and tune them: see
// "Tuning the thresholds" in the README. KEYSTONE_RELEVANCE_FLOOR and KEYSTONE_BLOCK_MARGIN
// override them without a rebuild.
type Thresholds struct {
	Floor  float64
	Margin float64
}

// DefaultThresholds are the values the product ships with, tuned on the demo documents.
//
// Floor 0.40 sits above the demo's unanswerable question (0.298) and above every other
// unanswerable probe except one near-topic bait (0.463), and below every answerable demo
// question (the weakest scored 0.477). It is a trade: some questions that do have an answer
// score under it, and are reported as no match (about 1 in 9 of the labelled questions).
// Margin 0.05 sits well below the smallest blocked margin measured and well above the largest
// margin of a question that must be answered although a forbidden document is also relevant.
var DefaultThresholds = Thresholds{Floor: 0.40, Margin: 0.05}

// Decide is plain code over two numbers, and nothing else. bestPermitted is the
// best similarity among documents the asker's team may read, bestForbidden the
// best among documents it may not; pass NoScore when there are none.
//
//	blocked:  the best forbidden passage clears the floor AND beats the best
//	          permitted one by at least the margin.
//	answered: otherwise, the best permitted passage clears the floor.
//	no_match: otherwise.
//
// Blocked is checked first: if the document that answers the question is one the
// team cannot read, a weak permitted passage must not be answered in its place.
func Decide(th Thresholds, bestPermitted, bestForbidden float64) Outcome {
	if bestForbidden >= th.Floor && bestForbidden-bestPermitted >= th.Margin {
		return Blocked
	}
	if bestPermitted >= th.Floor {
		return Answered
	}
	return NoMatch
}
