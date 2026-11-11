package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"stardefender/internal/level"
)

func do(t *testing.T, h http.Handler, method, path string, body any, out any) int {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: bad JSON %q", method, path, rec.Body.String())
		}
	}
	return rec.Code
}

func newTestHandler(t *testing.T) (http.Handler, string) {
	dir := t.TempDir()
	st, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return newHandler(st, t.TempDir()), dir
}

func TestScores(t *testing.T) {
	h, dir := newTestHandler(t)
	type resp struct {
		Rank   int     `json:"rank"`
		Scores []Score `json:"scores"`
	}
	var r resp
	for _, s := range []int{500, 900, 700} {
		if c := do(t, h, "POST", "/api/scores", map[string]any{"name": " ace\x01 ", "score": s, "level": 2}, &r); c != 200 {
			t.Fatalf("post: %d", c)
		}
	}
	if r.Rank != 2 || len(r.Scores) != 3 || r.Scores[0].Score != 900 || r.Scores[2].Score != 500 {
		t.Fatalf("bad table: %+v", r)
	}
	if r.Scores[0].Name != "ace" {
		t.Errorf("name not cleaned: %q", r.Scores[0].Name)
	}
	for _, bad := range []map[string]any{
		{"name": "", "score": 5, "level": 1},
		{"name": "x", "score": -5, "level": 1},
		{"name": "x", "score": 5, "level": 99},
	} {
		if c := do(t, h, "POST", "/api/scores", bad, nil); c != 400 {
			t.Errorf("%v: got %d, want 400", bad, c)
		}
	}
	// persisted across restarts
	st2, _ := openStore(dir)
	if got := st2.topScores(10); len(got) != 3 || got[0].Score != 900 {
		t.Errorf("not persisted: %+v", got)
	}
}

func TestLevels(t *testing.T) {
	h, _ := newTestHandler(t)
	var list []level.Summary
	do(t, h, "GET", "/api/levels", nil, &list)
	if len(list) != 30 || !list[0].Builtin {
		t.Fatalf("want the 30 built-ins, got %d", len(list))
	}

	var l level.Level
	if c := do(t, h, "GET", "/api/levels/level-03", nil, &l); c != 200 || l.Name != "Minefield" {
		t.Fatalf("get builtin: %d %q", c, l.Name)
	}

	// Saving an edited built-in creates a new custom level.
	l.Name = "My Minefield"
	var saved level.Level
	if c := do(t, h, "POST", "/api/levels", l, &saved); c != 200 {
		t.Fatalf("save: %d", c)
	}
	if saved.ID == l.ID || saved.Builtin {
		t.Fatalf("built-in was overwritten: %+v", saved.Summary())
	}
	// Saving again with the new id updates in place.
	saved.Name = "Renamed"
	var again level.Level
	do(t, h, "POST", "/api/levels", saved, &again)
	do(t, h, "GET", "/api/levels", nil, &list)
	if again.ID != saved.ID || len(list) != 31 || list[30].Name != "Renamed" {
		t.Fatalf("update failed: %+v", list[len(list)-1])
	}

	bad := saved.Clone()
	bad.Terrain = nil
	if c := do(t, h, "POST", "/api/levels", bad, nil); c != 400 {
		t.Errorf("invalid level accepted: %d", c)
	}
	if c := do(t, h, "DELETE", "/api/levels/level-01", nil, nil); c != 403 {
		t.Errorf("deleting a built-in: %d", c)
	}
	if c := do(t, h, "DELETE", "/api/levels/"+saved.ID, nil, nil); c != 200 {
		t.Errorf("delete: %d", c)
	}
	if c := do(t, h, "GET", "/api/levels/"+saved.ID, nil, nil); c != 404 {
		t.Errorf("deleted level still there: %d", c)
	}
}
