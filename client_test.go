package sesh_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/matthewmueller/sesh"
	"github.com/matthewmueller/sesh/mockstore"
)

func TestClient(t *testing.T) {
	is := is.New(t)
	type Data struct {
		Visits int
	}
	sessions := sesh.New[Data]()
	handler := sessions.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") == "present" {
			is.Equal(r.Cookies()[0].Name, "other")
		}
		data := sessions.FromRequest(r)
		data.Visits++
		w.Write([]byte(strconv.Itoa(data.Visits)))
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := sessions.Client(Data{Visits: 4})
	is.NoErr(err)
	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	is.NoErr(err)
	request.Header.Set("X-Test", "present")
	request.AddCookie(&http.Cookie{Name: "other", Value: "value"})
	request.AddCookie(&http.Cookie{Name: sessions.Cookie.Name, Value: "wrong"})
	for _, want := range []string{"5", "6"} {
		response, err := client.Do(request)
		is.NoErr(err)
		body, err := io.ReadAll(response.Body)
		is.NoErr(err)
		is.NoErr(response.Body.Close())
		is.Equal(string(body), want)
	}
	is.Equal(request.Cookies()[1].Value, "wrong")

	other, err := sessions.Client(Data{Visits: 10})
	is.NoErr(err)
	response, err := other.Get(server.URL)
	is.NoErr(err)
	body, err := io.ReadAll(response.Body)
	is.NoErr(err)
	is.NoErr(response.Body.Close())
	is.Equal(string(body), "11")
}

func TestTransportError(t *testing.T) {
	is := is.New(t)
	sessions := sesh.New[struct{}]()
	failure := errors.New("store unavailable")
	mock := mockstore.New()
	mock.MockUpsert = func(ctx context.Context, id string, data []byte, expiry time.Time) error {
		return failure
	}
	sessions.Store = mock
	transport, err := sessions.Transport(struct{}{})
	is.True(errors.Is(err, failure))
	is.Equal(transport, nil)
}
