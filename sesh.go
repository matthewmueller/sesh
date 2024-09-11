package sesh

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/matthewmueller/httpbuf"
)

// New session manager
func New[Data any]() *Manager[Data] {
	return &Manager[Data]{
		Cookie: &Cookie{
			Name:     "sid",
			HttpOnly: true,
			ExpireIn: time.Hour * 24 * 7,
			SameSite: http.SameSiteLaxMode,
			Path:     "/",
			Secure:   false,
			// TODO: need a way to be able to set a browser session cookie, yet still
			// load sessions from the store during that session
		},
		Store:        newMemoryStore(),
		Codec:        &gobCodec{},
		Now:          time.Now,
		Generate:     generateRandom,
		ErrorHandler: errorHandler,
	}
}

// Manager manages sessions
type Manager[Data any] struct {
	Cookie *Cookie
	Store  Store
	Codec  Codec

	// ErrorHandler is called when an error occurs in the middleware
	// Default is to return a 500 status code with the error message.
	ErrorHandler func(http.ResponseWriter, *http.Request, error)

	// Now is used to get the current time. This is useful for testing.
	Now func() time.Time

	// Generate is used to generate a new session id.
	Generate func() (string, error)
}

// Load the session from the store
func (m *Manager[Data]) Load(ctx context.Context, id string) (*Session[*Data], error) {
	raw, expiry, err := m.Store.Find(ctx, id)
	if err != nil {
		return nil, err
	}
	// Session not found or expired
	if raw == nil {
		return m.newSession(), nil
	} else if expiry.Before(m.Now()) {
		return m.newSession(), nil
	}
	// Session data found, decode it
	data := new(Data)
	if err := m.Codec.Decode(raw, &data); err != nil {
		return nil, err
	}
	return &Session[*Data]{
		ID:     id,
		Data:   data,
		Expiry: expiry,
	}, nil
}

func (m *Manager[Data]) newSession() *Session[*Data] {
	return &Session[*Data]{
		Data:   new(Data),
		Expiry: m.Now().Add(m.Cookie.ExpireIn),
	}
}

type Session[Data any] struct {
	ID     string // Will be empty if the session is new
	Data   Data
	Expiry time.Time
}

// generateRandom generates a random session ID.
func generateRandom() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (m *Manager[Data]) prepareSession(session *Session[*Data]) (err error) {
	if session.ID == "" {
		session.ID, err = m.Generate()
		if err != nil {
			return err
		}
	}
	if session.Expiry.IsZero() {
		session.Expiry = m.Now().Add(m.Cookie.ExpireIn)
	}
	return nil
}

// Save the session to the store
func (m *Manager[Data]) Save(ctx context.Context, session *Session[*Data]) (err error) {
	if err := m.prepareSession(session); err != nil {
		return err
	}
	return m.save(ctx, session)
}

func (m *Manager[Data]) save(ctx context.Context, session *Session[*Data]) (err error) {
	raw, err := m.Codec.Encode(session.Data)
	if err != nil {
		return err
	}
	return m.Store.Upsert(ctx, session.ID, raw, session.Expiry)
}

// Delete the session from the store
func (m *Manager[Data]) Delete(ctx context.Context, id string) (err error) {
	return m.Store.Delete(ctx, id)
}

type contextKey string

const sessionKey = contextKey("session")

func errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// Middleware for loading and saving sessions
func (m *Manager[Data]) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := m.Read(r)
		if err != nil {
			m.ErrorHandler(w, r, err)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), sessionKey, session))
		rw := httpbuf.Wrap(w)
		next.ServeHTTP(rw, r)
		if err := m.Write(w, r, session); err != nil {
			m.ErrorHandler(w, r, err)
			return
		}
		rw.Flush()
	})
}

// Request is the minimal interface required for loading cookies
type Request interface {
	Context() context.Context
	Cookie(name string) (*http.Cookie, error)
}

// ResponseWriter is the minimal interface required for setting cookies
type ResponseWriter interface {
	Header() http.Header
}

// Read the session from the request
func (m *Manager[Data]) Read(r Request) (session *Session[*Data], err error) {
	if session, ok := r.Context().Value(sessionKey).(*Session[*Data]); ok {
		return session, nil
	}
	cookie, err := r.Cookie(m.Cookie.Name)
	if err != nil {
		if !errors.Is(err, http.ErrNoCookie) {
			return nil, err
		}
		return m.newSession(), nil
	}
	return m.Load(r.Context(), cookie.Value)
}

// Write the session to the response
func (m *Manager[Data]) Write(w ResponseWriter, r Request, session *Session[*Data]) (err error) {
	if err := m.prepareSession(session); err != nil {
		return err
	}
	if err := m.save(r.Context(), session); err != nil {
		return err
	}
	cookie := &http.Cookie{
		Value:    session.ID,
		Expires:  session.Expiry,
		Name:     m.Cookie.Name,
		HttpOnly: m.Cookie.HttpOnly,
		SameSite: m.Cookie.SameSite,
		Path:     m.Cookie.Path,
	}
	if v := cookie.String(); v != "" {
		w.Header().Add("Set-Cookie", v)
	}
	return nil
}

// Session returns the session data from the request
func (m *Manager[Data]) Session(r Request) (session *Data) {
	s, ok := r.Context().Value(sessionKey).(*Session[*Data])
	if !ok {
		return new(Data)
	}
	return s.Data
}

// func (m *Manager)
