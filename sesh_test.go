package sesh_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"strconv"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/diff"
	"github.com/matthewmueller/sesh"
	"github.com/matthewmueller/sesh/mockstore"
	"golang.org/x/sync/errgroup"
)

func equal(t testing.TB, jar *cookiejar.Jar, h http.Handler, r *http.Request, expect string) {
	t.Helper()
	for _, cookie := range jar.Cookies(r.URL) {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	w := rec.Result()
	jar.SetCookies(r.URL, w.Cookies())
	dump, err := httputil.DumpResponse(w, true)
	if err != nil {
		if err.Error() != expect {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	diff.TestHTTP(t, string(dump), expect)
}

func futureDate() time.Time {
	return time.Date(2080, 1, 1, 0, 0, 0, 0, time.UTC)
}

func TestSetGetCookie(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	mux := http.NewServeMux()
	mux.Handle("/set", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "cookie_name", Value: "cookie_value"})
	}))
	mux.Handle("/get", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("cookie_name")
		is.NoErr(err)
		http.SetCookie(w, cookie)
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/set", nil)
	equal(t, jar, mux, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: cookie_name=cookie_value
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/get", nil)
	equal(t, jar, mux, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: cookie_name=cookie_value
	`)
}

func ExampleSession() {
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	type Data struct {
		User *User
	}
	sessions := sesh.New[Data]()
	router := http.NewServeMux()

	// Login a user
	router.HandleFunc("POST /sessions", func(w http.ResponseWriter, r *http.Request) {
		session := sessions.FromRequest(r)
		// Assumes we've loaded and authenticated the user
		session.User = &User{
			ID:   1,
			Name: "Alice",
		}
		http.Redirect(w, r, "/", http.StatusFound)
	})

	// Show the user if they're logged in
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		session := sessions.FromRequest(r)
		if session.User != nil {
			w.Write([]byte("Welcome " + session.User.Name))
			return
		}
		w.Write([]byte("Welcome!"))
	})

	handler := sessions.Middleware(router)
	http.ListenAndServe(":8080", handler)
}

func TestSession(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	type Data struct {
		Visits int
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	sessions.Generate = func() (string, error) {
		return "random_id", nil
	}
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := sessions.FromRequest(r)
		session.Visits++
		w.Write([]byte(strconv.Itoa(session.Visits)))
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		1
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		2
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		3
	`)
}

func TestLoadLegacySession(t *testing.T) {
	is := is.New(t)
	type LegacyData struct {
		Visits int
	}
	type Data struct {
		Visits int
		Name   string
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	var raw bytes.Buffer
	is.NoErr(gob.NewEncoder(&raw).Encode(LegacyData{Visits: 3}))
	expiry := futureDate().Add(time.Hour)
	is.NoErr(sessions.Store.Upsert(context.Background(), "legacy", raw.Bytes(), expiry))

	session, err := sessions.Load(context.Background(), "legacy")
	is.NoErr(err)
	is.Equal(session.ID, "legacy")
	is.Equal(session.Data.Visits, 3)
	is.Equal(session.Data.Name, "")
	is.Equal(session.Expiry, expiry)
	is.NoErr(sessions.Save(context.Background(), session))
	reloaded, err := sessions.Load(context.Background(), "legacy")
	is.NoErr(err)
	is.Equal(reloaded.Data.Visits, 3)
}

func TestLoadAddedDataField(t *testing.T) {
	is := is.New(t)
	type OldData struct {
		Visits int
	}
	type Data struct {
		Visits int
		Name   string
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	var raw bytes.Buffer
	is.NoErr(gob.NewEncoder(&raw).Encode(struct {
		Data    *OldData
		Flashes sesh.Flashes
	}{
		Data: &OldData{Visits: 3},
	}))
	is.NoErr(sessions.Store.Upsert(context.Background(), "existing", raw.Bytes(), futureDate().Add(time.Hour)))

	session, err := sessions.Load(context.Background(), "existing")
	is.NoErr(err)
	is.Equal(session.ID, "existing")
	is.Equal(session.Data.Visits, 3)
	is.Equal(session.Data.Name, "")
}

func TestGobPayloadAddedField(t *testing.T) {
	is := is.New(t)
	type Data struct {
		Visits int
	}
	sessions := sesh.New[Data]()
	raw, err := sessions.Codec.Encode(struct {
		Data    *Data
		Flashes sesh.Flashes
	}{
		Data:    &Data{Visits: 3},
		Flashes: sesh.Flashes{"notice": {"saved"}},
	})
	is.NoErr(err)

	var expanded struct {
		Data    *Data
		Flashes sesh.Flashes
		Extra   string
	}
	is.NoErr(sessions.Codec.Decode(raw, &expanded))
	is.Equal(expanded.Data.Visits, 3)
	is.Equal(expanded.Flashes.Get("notice"), "saved")
	is.Equal(expanded.Extra, "")
}

func TestLoadCorruptSession(t *testing.T) {
	is := is.New(t)
	type Data struct {
		Visits int
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	is.NoErr(sessions.Store.Upsert(context.Background(), "corrupt", []byte("invalid gob"), futureDate().Add(time.Hour)))

	session, err := sessions.Load(context.Background(), "corrupt")
	is.NoErr(err)
	is.Equal(session.ID, "")
	is.Equal(session.Data.Visits, 0)
	is.Equal(session.Expiry, futureDate().Add(sessions.Cookie.ExpireIn))
}

func TestConcurrency(t *testing.T) {
	is := is.New(t)
	type Data struct {
		Visits int
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := sessions.FromRequest(r)
		session.Visits++
		w.Write([]byte(strconv.Itoa(session.Visits)))
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	eg := errgroup.Group{}
	for i := 0; i < 100; i++ {
		eg.Go(func() error {
			jar, err := cookiejar.New(nil)
			is.NoErr(err)
			client := &http.Client{
				Jar: jar,
			}
			res, err := client.Get(server.URL)
			is.NoErr(err)
			res.Header.Del("Date")
			is.True(res.Header.Get("Set-Cookie") != "")
			res.Header.Del("Set-Cookie")
			body, err := httputil.DumpResponse(res, true)
			is.NoErr(err)
			diff.TestHTTP(t, string(body), `
				HTTP/1.1 200 OK
				Content-Length: 1
				Content-Type: text/plain; charset=utf-8

				1
			`)
			res, err = client.Get(server.URL)
			is.NoErr(err)
			res.Header.Del("Date")
			is.True(res.Header.Get("Set-Cookie") != "")
			res.Header.Del("Set-Cookie")
			body, err = httputil.DumpResponse(res, true)
			is.NoErr(err)
			diff.TestHTTP(t, string(body), `
				HTTP/1.1 200 OK
				Content-Length: 1
				Content-Type: text/plain; charset=utf-8

				2
			`)
			res, err = client.Get(server.URL)
			is.NoErr(err)
			res.Header.Del("Date")
			is.True(res.Header.Get("Set-Cookie") != "")
			res.Header.Del("Set-Cookie")
			body, err = httputil.DumpResponse(res, true)
			is.NoErr(err)
			diff.TestHTTP(t, string(body), `
				HTTP/1.1 200 OK
				Content-Length: 1
				Content-Type: text/plain; charset=utf-8

				3
			`)
			return nil
		})
	}
	is.NoErr(eg.Wait())
}

func TestSessionNested(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	type User struct {
		ID int `json:"id"`
	}
	type Data struct {
		Visits int   `json:"visits"`
		User   *User `json:"user"`
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	sessions.Generate = func() (string, error) {
		return "random_id", nil
	}
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := sessions.FromRequest(r)
		switch session.Visits {
		case 0:
			session.Visits++
		case 1:
			session.Visits++
			session.User = &User{ID: 1}
		case 2:
			session.Visits++
			session.User.ID++
		case 3:
			session.Visits++
			session.User = nil
		case 4:
			session.Visits++
			session.User = &User{ID: 1}
		}
		b := new(bytes.Buffer)
		b.WriteString(strconv.Itoa(session.Visits))
		b.WriteString(":")
		if session.User != nil {
			b.WriteString(strconv.Itoa(session.User.ID))
		}
		w.Write(b.Bytes())
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		1:
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		2:1
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		3:2
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		4:
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		5:1
	`)
}

func TestDelete(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	type Data struct {
		Visits int
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	sessions.Generate = func() (string, error) {
		return "random_id", nil
	}
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := sessions.FromRequest(r)
		switch session.Visits {
		case 0:
			session.Visits++
		case 1:
			session.Visits++
		case 2:
			*session = Data{}
		}
		w.Write([]byte(strconv.Itoa(session.Visits)))
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		1
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		2
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		0
	`)
}

func TestCustomFlash(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	type Data struct {
		Flashes []string
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	sessions.Generate = func() (string, error) {
		return "random_id", nil
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			session := sessions.FromRequest(r)
			session.Flashes = append(session.Flashes, "validation error")
			http.Redirect(w, r, "/", http.StatusSeeOther)
		case http.MethodGet:
			session := sessions.FromRequest(r)
			for _, flash := range session.Flashes {
				w.Write([]byte(flash))
			}
			session.Flashes = nil
		}
	}))

	handler := sessions.Middleware(mux)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 303 See Other
		Connection: close
		Location: /
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		validation error
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax
	`)
}

func TestNil(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	type Data struct {
		Visits int
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	sessions.Generate = func() (string, error) {
		return "random_id", nil
	}
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := sessions.FromRequest(r)
		session.Visits++
		w.Write([]byte(strconv.Itoa(session.Visits)))
		// Nil doesn't impact the session
		session = nil
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		1
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		2
	`)
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 200 OK
		Connection: close
		Set-Cookie: sid=random_id; Path=/; Expires=Mon, 08 Jan 2080 00:00:00 GMT; HttpOnly; SameSite=Lax

		3
	`)
}

func TestErrorHandler(t *testing.T) {
	is := is.New(t)
	jar, err := cookiejar.New(nil)
	is.NoErr(err)
	type Data struct {
		Visits int
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	sessions.Generate = func() (string, error) {
		return "random_id", nil
	}
	mock := mockstore.New()
	mock.MockUpsert = func(ctx context.Context, id string, data []byte, expiry time.Time) error {
		return errors.New("oh noz")
	}
	sessions.Store = mock
	called := 0
	sessions.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		called++
		is.Equal(err.Error(), "oh noz")
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := sessions.FromRequest(r)
		session.Visits++
		w.Write([]byte(strconv.Itoa(session.Visits)))
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	equal(t, jar, handler, req, `
		HTTP/1.1 500 Internal Server Error
		Connection: close
		Content-Type: text/plain; charset=utf-8
		X-Content-Type-Options: nosniff

		oh noz
	`)
}
