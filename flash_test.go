package sesh_test

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/matryer/is"
	"github.com/matthewmueller/sesh"
)

func TestFlashes(t *testing.T) {
	is := is.New(t)
	flashes := sesh.Flashes{}
	is.Equal(flashes.Get("missing"), "")
	is.Equal(len(flashes.Values("missing")), 0)
	flashes.Add("error", "Name is required")
	flashes.Add("error", "Email is required")
	is.Equal(flashes.Get("error"), "Name is required")
	is.Equal(flashes.Values("error"), []string{"Name is required", "Email is required"})
	flashes.Set("error", "Try again")
	is.Equal(flashes.Values("error"), []string{"Try again"})
	flashes.Set("Error", "Case matters")
	is.Equal(flashes.Get("error"), "Try again")
	is.Equal(flashes.Get("Error"), "Case matters")
	flashes.Del("error")
	is.Equal(flashes.Get("error"), "")
	flashes.Clear()
	is.Equal(len(flashes), 0)
	var empty sesh.Flashes
	is.Equal(empty.Get("missing"), "")
	is.Equal(len(empty.Values("missing")), 0)
	empty.Del("missing")
	empty.Clear()
}

func flashRequest(t testing.TB, jar *cookiejar.Jar, handler http.Handler, path string) {
	t.Helper()
	is := is.New(t)
	r := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
	for _, cookie := range jar.Cookies(r.URL) {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	is.Equal(w.Code, http.StatusOK)
	jar.SetCookies(r.URL, w.Result().Cookies())
}

func TestFlashLifecycle(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	sessions := sesh.New[struct{}]()
	sessions.Now = futureDate
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			is.Equal(len(sessions.Flash.All(r)), 0)
			sessions.Flash.Set(r, "notice", "Board created")
			sessions.Flash.Add(r, "error", "Name is required")
			sessions.Flash.Add(r, "error", "Email is required")
			is.Equal(sessions.Flash.Get(r, "notice"), "")
			is.Equal(len(sessions.Flash.All(r)), 0)
		case "/read":
			is.Equal(sessions.Flash.Get(r, "notice"), "Board created")
			is.Equal(sessions.Flash.Get(r, "notice"), "Board created")
			is.Equal(sessions.Flash.Get(r, "error"), "Name is required")
			is.Equal(sessions.Flash.All(r), sesh.Flashes{
				"notice": {"Board created"},
				"error":  {"Name is required", "Email is required"},
			})
		case "/empty":
			is.Equal(sessions.Flash.Get(r, "notice"), "")
			is.Equal(len(sessions.Flash.All(r)), 0)
		}
	}))
	flashRequest(t, jar, handler, "/set")
	flashRequest(t, jar, handler, "/read")
	flashRequest(t, jar, handler, "/empty")
	flashRequest(t, jar, handler, "/set")
	flashRequest(t, jar, handler, "/unread")
	flashRequest(t, jar, handler, "/empty")
}

func TestFlashPending(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	sessions := sesh.New[struct{}]()
	sessions.Now = futureDate
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			sessions.Flash.Set(r, "notice", "Current")
		case "/replace":
			sessions.Flash.Add(r, "notice", "Next")
			is.Equal(sessions.Flash.Get(r, "notice"), "Current")
		case "/delete":
			is.Equal(sessions.Flash.Get(r, "notice"), "Next")
			sessions.Flash.Set(r, "notice", "Deleted")
			sessions.Flash.Set(r, "other", "Preserved")
			sessions.Flash.Del(r, "notice")
			is.Equal(sessions.Flash.Get(r, "notice"), "Next")
		case "/clear":
			is.Equal(sessions.Flash.Get(r, "notice"), "")
			is.Equal(sessions.Flash.Get(r, "other"), "Preserved")
			sessions.Flash.Set(r, "other", "Deleted")
			sessions.Flash.Add(r, "error", "Also deleted")
			sessions.Flash.Clear(r)
			is.Equal(sessions.Flash.Get(r, "other"), "Preserved")
		case "/empty":
			is.Equal(len(sessions.Flash.All(r)), 0)
		}
	}))
	flashRequest(t, jar, handler, "/set")
	flashRequest(t, jar, handler, "/replace")
	flashRequest(t, jar, handler, "/delete")
	flashRequest(t, jar, handler, "/clear")
	flashRequest(t, jar, handler, "/empty")
}

func TestFlashAllClone(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	sessions := sesh.New[struct{}]()
	sessions.Now = futureDate
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/set" {
			sessions.Flash.Add(r, "error", "First")
			sessions.Flash.Add(r, "error", "Second")
			return
		}
		flashes := sessions.Flash.All(r)
		flashes["error"][0] = "Changed"
		flashes.Set("notice", "Added")
		flashes.Del("error")
		is.Equal(sessions.Flash.Get(r, "error"), "First")
		is.Equal(sessions.Flash.All(r), sesh.Flashes{"error": {"First", "Second"}})
	}))
	flashRequest(t, jar, handler, "/set")
	flashRequest(t, jar, handler, "/read")
}

type flashJSONCodec struct{}

var _ sesh.Codec = flashJSONCodec{}

func (flashJSONCodec) Encode(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (flashJSONCodec) Decode(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func TestFlashCodec(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	type Data struct {
		Visits  int
		Flashes string
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	sessions.Codec = flashJSONCodec{}
	writer := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := sessions.FromRequest(r)
		data.Visits = 42
		data.Flashes = "Application data"
		sessions.Flash.Set(r, "notice", "Saved")
	}))
	flashRequest(t, jar, writer, "/set")
	// A separate manager must recover both kinds of state from the shared store.
	reader := sesh.New[Data]()
	reader.Now = futureDate
	reader.Store = sessions.Store
	reader.Codec = flashJSONCodec{}
	handler := reader.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := reader.FromRequest(r)
		is.Equal(data.Flashes, "Application data")
		if r.URL.Path == "/read" {
			is.Equal(data.Visits, 42)
			is.Equal(reader.Flash.Get(r, "notice"), "Saved")
			data.Visits++
		} else {
			is.Equal(data.Visits, 43)
			is.Equal(len(reader.Flash.All(r)), 0)
		}
	}))
	flashRequest(t, jar, handler, "/read")
	flashRequest(t, jar, handler, "/empty")
}

func TestFlashOutsideMiddleware(t *testing.T) {
	is := is.New(t)
	sessions := sesh.New[struct{}]()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	sessions.Flash.Set(r, "notice", "Ignored")
	sessions.Flash.Add(r, "error", "Ignored")
	sessions.Flash.Del(r, "notice")
	sessions.Flash.Clear(r)
	is.Equal(sessions.Flash.Get(r, "notice"), "")
	is.Equal(len(sessions.Flash.All(r)), 0)
}
