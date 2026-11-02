package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func (w *askWorld) docPath(doc string) string {
	return "/api/admin/documents/" + strconv.FormatInt(w.docs[doc], 10)
}

// Deleting a document removes it, its upload record and every logged question whose answer
// cited it, keeps every other logged question, and the deleted text can never be answered from
// again. It runs through the real endpoints.
func TestDeletingADocumentRemovesItsRecordAndTheQuestionsThatCitedIt(t *testing.T) {
	w := newAskWorld(t, askOpts{})
	w.seed()
	adm := w.tokens["ADM-001"]
	const retainer = "What discount can I offer a client who commits to a twelve month retainer?"

	// Sales asks about the playbook (answered, cites it); Delivery asks too and is refused (no
	// citation, but a logged row); HR asks about leave (answered, cites the leave policy).
	if ev, _ := w.ask(w.tokens["SAL-001"], retainer); last(ev).str("outcome") != "answered" {
		t.Fatalf("sales should be answered: %v", last(ev).Data)
	}
	if ev, _ := w.ask(w.tokens["DEL-001"], retainer); last(ev).str("outcome") != "blocked" {
		t.Fatalf("delivery should be blocked: %v", last(ev).Data)
	}
	if ev, _ := w.ask(w.tokens["HR-001"], "How many days of annual leave do I get?"); last(ev).str("outcome") != "answered" {
		t.Fatalf("hr should be answered: %v", last(ev).Data)
	}
	if got := len(w.logRows()); got != 3 {
		t.Fatalf("want 3 logged questions before the delete, got %d", got)
	}

	code, body := w.adminCall(adm, "DELETE", w.docPath("playbook"), nil)
	if code != http.StatusOK {
		t.Fatalf("delete: %d %v", code, body)
	}
	if body["questions_removed"] != float64(1) || body["passages"] != float64(1) {
		t.Errorf("want 1 passage and 1 question removed, got %v", body)
	}
	msg, _ := body["message"].(string)
	if !strings.Contains(msg, "Deleted Pricing Playbook CANARYSALES") || !strings.Contains(msg, "1 passage") || !strings.Contains(msg, "1 logged question") || !strings.Contains(msg, "can no longer use it") {
		t.Errorf("the message must say what was deleted and what went with it: %q", msg)
	}

	// Gone from the list and from the ingestion log.
	_, list := w.adminCall(adm, "GET", "/api/admin/documents", nil)
	for _, d := range list["documents"].([]any) {
		if d.(map[string]any)["id"] == float64(w.docs["playbook"]) {
			t.Error("the deleted document is still listed")
		}
	}
	if len(list["documents"].([]any)) != 2 {
		t.Errorf("two documents should remain: %v", list["documents"])
	}
	_, logs := w.adminCall(adm, "GET", "/api/admin/ingestion-log", nil)
	for _, e := range logs["entries"].([]any) {
		if e.(map[string]any)["document_id"] == float64(w.docs["playbook"]) {
			t.Error("the deleted document is still in the ingestion log")
		}
	}
	// The query log lost the row that cited it and kept the other two (the refusal never cited it).
	rows := w.logRows()
	if len(rows) != 2 {
		t.Fatalf("want 2 logged questions after the delete (the refusal and the leave question), got %d", len(rows))
	}
	for _, r := range rows {
		if r.EmployeeNumber == "SAL-001" {
			t.Error("the question whose answer cited the deleted document is still logged")
		}
	}
	// The viewer cannot open it, for the team that could read it.
	if code, _ := w.adminCall(w.tokens["SAL-001"], "GET", "/api/documents/"+strconv.FormatInt(w.docs["playbook"], 10), nil); code != http.StatusNotFound {
		t.Errorf("opening the deleted document: want 404, got %d", code)
	}
	// Asking again never reaches the deleted text, and nothing is answered from it.
	w.model.reset()
	ev, _ := w.ask(w.tokens["SAL-001"], retainer)
	if last(ev).str("outcome") == "answered" || strings.Contains(w.model.everything(), "CANARYSALES") {
		t.Errorf("after the delete nothing may be answered from, or sent to the model from, the document: %v", last(ev).Data)
	}
	// Deleting it again, or an unknown id, is a calm 404.
	if code, _ := w.adminCall(adm, "DELETE", w.docPath("playbook"), nil); code != http.StatusNotFound {
		t.Errorf("deleting twice: want 404, got %d", code)
	}
	if code, _ := w.adminCall(adm, "DELETE", "/api/admin/documents/999999", nil); code != http.StatusNotFound {
		t.Errorf("unknown id: want 404, got %d", code)
	}
}

// A read-only deployment refuses the delete and changes nothing.
func TestAReadOnlyDeploymentRefusesToDeleteADocument(t *testing.T) {
	w := newAskWorld(t, askOpts{readOnly: true})
	w.seed()
	code, body := w.adminCall(w.tokens["ADM-001"], "DELETE", w.docPath("playbook"), nil)
	if code != http.StatusForbidden || !strings.Contains(strings.ToLower(body["error"].(map[string]any)["code"].(string)), "read_only") {
		t.Fatalf("delete while read-only: %d %v", code, body)
	}
	_, list := w.adminCall(w.tokens["ADM-001"], "GET", "/api/admin/documents", nil)
	if len(list["documents"].([]any)) != 3 {
		t.Errorf("nothing may be deleted while read-only: %v", list["documents"])
	}
}
