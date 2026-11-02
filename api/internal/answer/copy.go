package answer

import (
	"fmt"
	"strings"
	"time"
)

// This file holds every sentence the Ask flow says without the model. They are
// static templates. The refusal in particular is never written by the model, and
// the model is not called for a blocked question at all.

// RefusalBlocked is what an employee sees when the best-scoring match for a question
// is in a document their team cannot read. It says exactly that, and no more: it does
// not claim the document answers the question, only that it matched best. It names
// the team that holds the document, and never its title or anything it says.
func RefusalBlocked(yourTeam string, holdingTeams []string) string {
	holders, verb := "another team", "holds"
	if len(holdingTeams) > 0 {
		holders = joinNames(holdingTeams)
		if len(holdingTeams) > 1 {
			verb = "hold"
		}
	}
	return fmt.Sprintf("This is outside your team's access. The closest match to this question is in a document that %s %s and %s cannot read, so nothing from it was used here. To have Keystone use it, ask an admin to extend that document's access to your team.",
		holders, verb, yourTeam)
}

// NoMatch is what an employee sees when nothing in what their team can read
// answers the question. The model is not called: there is nothing to ground an
// answer in, and a model asked anyway would be tempted to invent one.
const NoMatch = "Keystone could not find an answer to this in the documents your team can read. Nothing relevant came back, so nothing was written. Try rewording the question, or ask an admin whether the document you need has been added for your team."

// Waiting is shown while the embeddings service is rate limited. It is correct
// behaviour on a plan with a low request allowance, so it says what is happening
// and roughly how long, and does not read as a fault.
func Waiting(attempt, maxAttempts int, wait time.Duration) string {
	secs := int(wait.Round(time.Second) / time.Second)
	if attempt <= 1 {
		return fmt.Sprintf("Keystone is waiting its turn. The search service behind it allows only a few requests a minute on this plan, and this minute's are used up. Nothing has gone wrong and your question is still in line. Keystone will try again in about %d seconds, and this usually clears within a minute.", secs)
	}
	remaining := (maxAttempts - attempt) * secs
	return fmt.Sprintf("Still waiting its turn. The limit has not reset yet. Keystone will try again in about %d seconds and will keep trying for up to about %d seconds more. Your question is still in line.", secs, remaining)
}

// Queued is shown when another visitor's question is being searched and this one is next in
// line. Only one search runs at a time, because the search service allows a few requests a minute.
const Queued = "Keystone is waiting its turn. Another question is being searched first, and the search service behind Keystone allows only a few requests a minute on this plan. Your question is next in line. Nothing has gone wrong, and this usually takes under a minute."

// Errors the stream can end with.
const (
	EmbeddingsBusy        = "Keystone waited as long as it reasonably can and the search service is still at its limit. Nothing was lost and nothing was answered. Ask again in a minute."
	EmbeddingsUnreachable = "Keystone could not reach the search service just now. Nothing was answered. Try again in a moment."
	AnswerFailed          = "Keystone found the passages but could not write the answer just now. Nothing was lost. Ask again in a moment."
	AnswerFailedPartway   = "The answer stopped partway because Keystone lost its connection to the writing service. What is shown above is incomplete. Ask again in a moment."
)

func joinNames(names []string) string {
	switch len(names) {
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
