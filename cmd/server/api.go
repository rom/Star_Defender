package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"stardefender/internal/level"
)

func newHandler(st *store, webDir string) http.Handler {
	mux := http.NewServeMux()
	a := &api{st}
	mux.HandleFunc("GET /api/scores", a.getScores)
	mux.HandleFunc("POST /api/scores", a.postScore)
	mux.HandleFunc("GET /api/levels", a.listLevels)
	mux.HandleFunc("POST /api/levels", a.saveLevel)
	mux.HandleFunc("GET /api/levels/{id}", a.getLevel)
	mux.HandleFunc("DELETE /api/levels/{id}", a.deleteLevel)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotFound, "no such endpoint")
	})

	files := http.FileServer(http.Dir(webDir))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The client is rebuilt often and is small; always revalidate.
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		files.ServeHTTP(w, r)
	}))
	return mux
}

type api struct{ st *store }

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	reply(w, code, map[string]string{"error": msg})
}

func internal(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	fail(w, http.StatusInternalServerError, "internal error")
}

// decode reads a size-limited JSON body into v.
func decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func (a *api) getScores(w http.ResponseWriter, r *http.Request) {
	n := 10
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= maxScores {
		n = v
	}
	reply(w, http.StatusOK, a.st.topScores(n))
}

func (a *api) postScore(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
		Level int    `json:"level"`
	}
	if !decode(w, r, 4<<10, &in) {
		return
	}
	name := level.CleanText(in.Name, 12)
	if name == "" {
		fail(w, http.StatusBadRequest, "name required")
		return
	}
	if in.Score <= 0 || in.Score > maxScoreValue || in.Level < 1 || in.Level > len(level.Builtin()) {
		fail(w, http.StatusBadRequest, "invalid score")
		return
	}
	rank, err := a.st.addScore(Score{Name: name, Score: in.Score, Level: in.Level, Time: time.Now().Unix()})
	if err != nil {
		internal(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]any{"rank": rank, "scores": a.st.topScores(10)})
}

func (a *api) listLevels(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, a.st.listLevels())
}

func (a *api) getLevel(w http.ResponseWriter, r *http.Request) {
	l, err := a.st.getLevel(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	reply(w, http.StatusOK, l)
}

func (a *api) saveLevel(w http.ResponseWriter, r *http.Request) {
	var l level.Level
	if !decode(w, r, 256<<10, &l) {
		return
	}
	l.Name = level.CleanText(l.Name, 32)
	l.Author = level.CleanText(l.Author, 24)
	if err := l.Validate(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	saved, err := a.st.saveLevel(l)
	switch {
	case errors.Is(err, errFull):
		fail(w, http.StatusInsufficientStorage, err.Error())
	case err != nil:
		internal(w, err)
	default:
		reply(w, http.StatusOK, saved)
	}
}

func (a *api) deleteLevel(w http.ResponseWriter, r *http.Request) {
	err := a.st.deleteLevel(r.PathValue("id"))
	switch {
	case errors.Is(err, errNotFound):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, errBuiltin):
		fail(w, http.StatusForbidden, err.Error())
	case err != nil:
		internal(w, err)
	default:
		reply(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
