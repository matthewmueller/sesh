# Sesh

[![Go Reference](https://pkg.go.dev/badge/github.com/matthewmueller/sesh.svg)](https://pkg.go.dev/github.com/matthewmueller/sesh)

A minimal, type-safe, pluggable session manager for Go. A viable alternative to [gorilla/sessions](http://github.com/gorilla/sessions).

## Features

- Type-safe, minimal API using Go 1.18+ Generics
- Easy-to-use middleware design
- Pluggable session storage
- Doesn't break `http.Flusher`

## Example

```go
type User struct {
  ID   int    `json:"id"`
  Name string `json:"name"`
}
type Data struct {
  User *User
}

// Initialize the session manager
sessions := sesh.New[Data]()

// Setup the router
router := http.NewServeMux()

// Login a user
router.HandleFunc("POST /sessions", func(w http.ResponseWriter, r *http.Request) {
  // Get the session from context
  session := sessions.Session(r)

  // Assumes we've loaded and authenticated the user
  session.User = &User{
    ID:   1,
    Name: "Alice",
  }

  http.Redirect(w, r, "/", http.StatusFound)
})

// Show the user if they're logged in
router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
  // Get the session from context
  session := sessions.Session(r)

  // Logged in state
  if session.User != nil {
    w.Write([]byte("Welcome " + session.User.Name))
    return
  }

  // Logged out state
  w.Write([]byte("Welcome!"))
})

// Automatically find and update the session on each request
handler := sessions.Middleware(router)

// Listen on :8080
http.ListenAndServe(":8080", handler)
```

## Install

```sh
go get github.com/matthewmueller/sesh
```

## Stores

- **Memory:** By default sesh initializes an in-memory store. These sessions will last until your server is restart.
- **SQLite 3:** [sqstore](./sqstore/) contains a SQLite 3 implementation for storing sessions in SQLite.
- **Mock:** [mockstore](./mockstore/) contains a mockable storage. This is primarily used for testing.

Missing a [Store](store.go)? Open a [PR](https://github.com/matthewmueller/sesh/pulls)!

## FAQ

### How does this compare to [gorilla/sessions](https://github.com/gorilla/sessions)?

This library is newer, so it has a more modern API. It's also much less battle-tested. Gorilla has many more session stores.

In gorilla, you'll typically get and set sessions like this:

```go
func Handler(w http.ResponseWriter, r *http.Request) {
  // Get a session. We're ignoring the error resulted from decoding an
  // existing session: Get() always returns a session, even if empty.
  session, _ := store.Get(r, "session-name")
  // Set some session values.
  session.Values["foo"] = "bar"
  session.Values[42] = 43
  // Save it before we write to the response/return from the handler.
  err := session.Save(r, w)
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }
}
```

Here's what that looks like in sesh. You'll notice a bit less verbosity, along with not needing to save the session at the end:

```go
type Data struct {
  Foo string
  FortyTwo int
}

var sessions = sesh.New[Data]()

func Handler(w http.ResponseWriter, r *http.Request) {
  session := sessions.Session(r)
  // Set some session values.
  session.Foo = "bar"
  session.FortyTwo = 42
```

### How does this library compare to [alexedwards/scs](https://github.com/alexedwards/scs)?

Sesh was a successful experiment in trying get a type-safe session store. I also wanted a smaller API surface. As with [gorilla/sessions](https://github.com/gorilla/sessions), scs is much more battle-tested and has a larger set of session stores.

The libraries share a similar API:

```go
func main() {
	// Initialize a new session manager
	sessionManager = scs.New()

	mux := http.NewServeMux()
	mux.HandleFunc("/put", putHandler)
	mux.HandleFunc("/get", getHandler)

	// Setup the session middleware
	http.ListenAndServe(":4000", sessionManager.LoadAndSave(mux))
}

func putHandler(w http.ResponseWriter, r *http.Request) {
	// Store a new key and value in the session data.
	sessionManager.Put(r.Context(), "message", "Hello from a session!")
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	// Use the GetString helper to retrieve the string value associated with a
	// key. The zero value is returned if the key does not exist.
	msg := sessionManager.GetString(r.Context(), "message")
	io.WriteString(w, msg)
}
```

While sesh would look like:

```go
type Data struct {
  Message string
}

func main() {
	// Initialize a new session manager
	sessionManager = sesh[Data].New()

	mux := http.NewServeMux()
	mux.HandleFunc("/put", putHandler)
	mux.HandleFunc("/get", getHandler)

  // Setup the session middleware
	http.ListenAndServe(":4000", sessionManager.Middleware(mux))
}

func putHandler(w http.ResponseWriter, r *http.Request) {
	// Store a new key and value in the session data.
  session := sessionManager.Session(r)
  session.Message = "Hello from a session!"
}

func getHandler(w http.ResponseWriter, r *http.Request) {
  session := sessionManager.Session(r)
	msg := session.Message
	io.WriteString(w, msg)
}
```

### Does it support a stateless cookie store?

Similar to [scs](https://github.com/alexedwards/scs), there is no store for saving the session within a cookie. A stateless session needs a different `Store` interface because it's tied to HTTP and needs to live within the request-response lifecycle.

I thought I'd miss this capability, but it turns out you get a lot of nice features if you store your sessions externally. You get the ability to:

- Tie much more data to a session
- Clear all sessions (e.g. log everyone out)
- Manipulate sessions outside of HTTP (e.g. workers)

## Thanks

- Alex Edwards ([@alexedwards](https://github.com/alexedwards)) for creating [scs](https://github.com/alexedwards/scs), which was a big inspiration for this project.

## Contributions

We welcome all contributions! Pull requests, bug reports and features requests are all appreciated.

If you have an idea or are unsure how to contribute, open an issue!

## Contributors

- Matt Mueller ([@mattmueller](https://twitter.com/mattmueller))

## License

MIT
