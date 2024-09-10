package sesh_test

import (
	"bytes"
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
		session := sessions.From(r)
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

func TestConcurrency(t *testing.T) {
	is := is.New(t)
	type Data struct {
		Visits int
	}
	sessions := sesh.New[Data]()
	sessions.Now = futureDate
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := sessions.From(r)
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
		session := sessions.From(r)
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
		session := sessions.From(r)
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

func TestFlash(t *testing.T) {
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
			session := sessions.From(r)
			session.Flashes = append(session.Flashes, "validation error")
			http.Redirect(w, r, "/", http.StatusSeeOther)
		case http.MethodGet:
			session := sessions.From(r)
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
