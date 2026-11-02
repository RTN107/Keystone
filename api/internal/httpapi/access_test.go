package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func (w *askWorld) adminCall(token, method, path string, body any) (int, map[string]any) {
	w.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, w.srv.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (w *askWorld) accessPath(doc string) string {
	return "/api/admin/documents/" + strconv.FormatInt(w.docs[doc], 10) + "/access"
}

func (w *askWorld) accessRow(doc, team string) (source string, found bool) {
	w.t.Helper()
	err := w.pool.QueryRow(context.Background(),
		`SELECT source FROM document_access WHERE document_id = $1 AND team_id = $2`, w.docs[doc], w.teams[team]).Scan(&source)
	return source, err == nil
}

// The claim the demo rests on, through the real endpoints and not a SQL helper: a refused
// question is answered when it is asked again after an admin extends the document, with no
// new embedding call, and is refused again once the extension is taken back.
func TestExtendingAccessThroughTheEndpointTurnsABlockedQuestionIntoAnAnswerAndBackAgain(t *testing.T) {
	w := newAskWorld(t, askOpts{})
	w.seed()
	q := "What discount can I offer a client who commits to a twelve month retainer?"
	del := w.tokens["DEL-001"]

	if events, _ := w.ask(del, q); last(events).str("outcome") != "blocked" {
		t.Fatalf("first ask must be blocked: %v", last(events).Data)
	}
	before := w.emb.QueryCalls()

	code, body := w.adminCall(w.tokens["ADM-001"], "POST", w.accessPath("playbook"), map[string]any{"team_id": w.teams["Delivery"]})
	if code != http.StatusCreated {
		t.Fatalf("extend: %d %v", code, body)
	}
	msg, _ := body["message"].(string)
	if !strings.Contains(msg, "Delivery can now read Pricing Playbook CANARYSALES") || !strings.Contains(msg, "next time Delivery asks") {
		t.Errorf("the message must name who gains access to what, and when: %q", msg)
	}
	var by int64
	var source string
	if err := w.pool.QueryRow(context.Background(),
		`SELECT source, granted_by FROM document_access WHERE document_id = $1 AND team_id = $2`, w.docs["playbook"], w.teams["Delivery"]).Scan(&source, &by); err != nil {
		t.Fatal(err)
	}
	if source != "extension" || by != w.ids["ADM-001"] {
		t.Errorf("row: source %q granted_by %d, want extension by the admin", source, by)
	}

	events, _ := w.ask(del, "  what DISCOUNT can I offer a client who commits to a twelve month retainer? ")
	done := last(events)
	if done.str("outcome") != "answered" {
		t.Fatalf("after the extension the re-ask must be answered: %v", done.Data)
	}
	if got := w.emb.QueryCalls() - before; got != 0 {
		t.Errorf("the re-ask made %d embedding calls, want 0", got)
	}

	// The documents list shows the extension apart from the original tags.
	code, list := w.adminCall(w.tokens["ADM-001"], "GET", "/api/admin/documents", nil)
	if code != http.StatusOK {
		t.Fatalf("list: %d", code)
	}
	var seen bool
	for _, d := range list["documents"].([]any) {
		doc := d.(map[string]any)
		if doc["id"] != float64(w.docs["playbook"]) {
			continue
		}
		for _, tg := range doc["teams"].([]any) {
			tag := tg.(map[string]any)
			if tag["name"] == "Delivery" && tag["source"] == "extension" {
				seen = true
			}
		}
	}
	if !seen {
		t.Error("the documents list should tag Delivery on the playbook as an extension")
	}

	code, body = w.adminCall(w.tokens["ADM-001"], "DELETE", w.accessPath("playbook")+"/"+strconv.FormatInt(w.teams["Delivery"], 10), nil)
	if code != http.StatusOK || !strings.Contains(body["message"].(string), "Delivery can no longer read") {
		t.Fatalf("revoke: %d %v", code, body)
	}
	if _, found := w.accessRow("playbook", "Delivery"); found {
		t.Error("the extension row should be gone")
	}
	w.model.reset()
	if events, _ := w.ask(del, q); last(events).str("outcome") != "blocked" {
		t.Errorf("after the revoke the question must be blocked again: %v", last(events).Data)
	}
	if strings.Contains(w.model.everything(), "CANARYSALES") {
		t.Error("the revoked document's text reached the model")
	}
}

func TestAccessChangesRefuseWhatTheyShouldAndSayWhy(t *testing.T) {
	w := newAskWorld(t, askOpts{})
	w.seed()
	adm := w.tokens["ADM-001"]
	team := func(n string) int64 { return w.teams[n] }
	rev := func(doc, tm string) string { return w.accessPath(doc) + "/" + strconv.FormatInt(team(tm), 10) }

	// Only admins.
	for _, c := range []struct{ method, path string }{{"POST", w.accessPath("playbook")}, {"DELETE", rev("playbook", "Delivery")}} {
		if code, _ := w.adminCall(w.tokens["DEL-001"], c.method, c.path, map[string]any{"team_id": team("Delivery")}); code != http.StatusForbidden {
			t.Errorf("%s %s as an employee: %d, want 403", c.method, c.path, code)
		}
		if code, _ := w.adminCall("", c.method, c.path, map[string]any{"team_id": team("Delivery")}); code != http.StatusUnauthorized {
			t.Errorf("%s %s with no token: %d, want 401", c.method, c.path, code)
		}
	}

	cases := []struct {
		name         string
		method, path string
		body         any
		code         int
		errCode      string
		says         string
	}{
		{"unknown document", "POST", "/api/admin/documents/999999/access", map[string]any{"team_id": team("Delivery")}, 404, "not_found", "does not exist"},
		{"a document id that is not a number", "POST", "/api/admin/documents/abc/access", map[string]any{"team_id": team("Delivery")}, 404, "not_found", "does not exist"},
		{"unknown team", "POST", w.accessPath("playbook"), map[string]any{"team_id": 999999}, 400, "unknown_team", "one of the three teams"},
		{"no team given", "POST", w.accessPath("playbook"), map[string]any{}, 400, "unknown_team", "one of the three teams"},
		{"already readable", "POST", w.accessPath("sop"), map[string]any{"team_id": team("Delivery")}, 409, "already_readable", "Delivery can already read"},
		{"revoking access given at upload", "DELETE", rev("sop", "Delivery"), nil, 409, "original_access", "when it was uploaded"},
		{"revoking what was never given", "DELETE", rev("playbook", "Delivery"), nil, 404, "no_extension", "has no extra access"},
	}
	for _, c := range cases {
		code, body := w.adminCall(adm, c.method, c.path, c.body)
		e, _ := body["error"].(map[string]any)
		if code != c.code || e["code"] != c.errCode {
			t.Errorf("%s: %d %v, want %d %s", c.name, code, body, c.code, c.errCode)
			continue
		}
		m, _ := e["message"].(string)
		if !strings.Contains(m, c.says) {
			t.Errorf("%s: the message should say %q: %q", c.name, c.says, m)
		}
		if c.code == 409 && !strings.Contains(m, "Nothing was changed") {
			t.Errorf("%s: a conflict should say nothing was changed: %q", c.name, m)
		}
	}

	// A refused revoke leaves the original access exactly as it was, and Delivery can still use its own document.
	if src, found := w.accessRow("sop", "Delivery"); !found || src != "ingest" {
		t.Errorf("Delivery's own access was touched: %q %v", src, found)
	}
	if events, _ := w.ask(w.tokens["DEL-001"], "What do I do when a priority ticket breaches its SLA?"); last(events).str("outcome") != "answered" {
		t.Errorf("Delivery should still be answered from its own document: %v", last(events).Data)
	}
	// Extending twice does not duplicate the row or change who granted it.
	w.adminCall(adm, "POST", w.accessPath("playbook"), map[string]any{"team_id": team("HR")})
	if code, _ := w.adminCall(adm, "POST", w.accessPath("playbook"), map[string]any{"team_id": team("HR")}); code != http.StatusConflict {
		t.Errorf("a second extension: %d, want 409", code)
	}
}
