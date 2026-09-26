package sesh

import (
	"context"
	"net/http"
)

// Transport sends requests with a session initialized with data.
func (m *Manager[Data]) Transport(data Data) (http.RoundTripper, error) {
	session := &Session[*Data]{Data: &data}
	if err := m.Save(context.Background(), session); err != nil {
		return nil, err
	}
	return &sessionTransport{cookie: http.Cookie{Name: m.Cookie.Name, Value: session.ID}}, nil
}

type sessionTransport struct {
	cookie http.Cookie
}

var _ http.RoundTripper = (*sessionTransport)(nil)

func (tr *sessionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	request := r.Clone(r.Context())
	request.Header.Del("Cookie")
	for _, cookie := range r.Cookies() {
		if cookie.Name != tr.cookie.Name {
			request.AddCookie(cookie)
		}
	}
	request.AddCookie(&tr.cookie)
	return http.DefaultTransport.RoundTrip(request)
}

func (m *Manager[Data]) Client(data Data) (*http.Client, error) {
	transport, err := m.Transport(data)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: transport,
	}, nil
}
