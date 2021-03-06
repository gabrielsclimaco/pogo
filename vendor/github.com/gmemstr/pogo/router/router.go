package router

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/gmemstr/pogo/admin"
	"github.com/gmemstr/pogo/auth"
	"github.com/gmemstr/pogo/common"
)

type NewConfig struct {
	Name        string
	Host        string
	Email       string
	Description string
	Image       string
	PodcastURL  string
}

// Handle takes multiple Handler and executes them in a serial order starting from first to last.
// In case, Any middle ware returns an error, The error is logged to console and sent to the user, Middlewares further up in chain are not executed.
func Handle(handlers ...common.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rc := &common.RouterContext{}
		for _, handler := range handlers {
			err := handler(rc, w, r)
			if err != nil {
				log.Printf("%v", err)

				w.Write([]byte(http.StatusText(err.StatusCode)))

				return
			}
		}
	})
}

func Init() *mux.Router {

	r := mux.NewRouter()

	// "Static" paths
	r.PathPrefix("/assets/").Handler(http.StripPrefix("/assets/", http.FileServer(http.Dir("assets/web/static"))))
	r.PathPrefix("/download/").Handler(http.StripPrefix("/download/", http.FileServer(http.Dir("podcasts"))))

	// Paths that require specific handlers
	r.Handle("/", Handle(
		rootHandler(),
	)).Methods("GET")

	r.Handle("/rss", Handle(
		rootHandler(),
	)).Methods("GET")

	r.Handle("/json", Handle(
		rootHandler(),
	)).Methods("GET")

	// Authenticated endpoints should be passed to BasicAuth()
	// first
	r.Handle("/admin", Handle(
		auth.RequireAuthorization(),
		adminHandler(),
	)).Methods("GET", "POST")

	r.Handle("/login", Handle(
		loginHandler(),
	)).Methods("GET", "POST")

	r.Handle("/admin/publish", Handle(
		auth.RequireAuthorization(),
		admin.CreateEpisode(),
	)).Methods("POST")

	r.Handle("/admin/delete", Handle(
		auth.RequireAuthorization(),
		admin.RemoveEpisode(),
	)).Methods("GET")

	r.Handle("/admin/css", Handle(
		auth.RequireAuthorization(),
		admin.CustomCss(),
	)).Methods("GET", "POST")

	r.Handle("/setup", Handle(
		serveSetup(),
	)).Methods("GET", "POST")

	return r
}

func loginHandler() common.Handler {
	return func(rc *common.RouterContext, w http.ResponseWriter, r *http.Request) *common.HTTPError {

		if _, err := auth.DecryptCookie(r); err == nil {
			http.Redirect(w, r, "/admin", http.StatusTemporaryRedirect)
			return nil
		}

		if r.Method == "GET" {
			w.Header().Set("Content-Type", "text/html")
			return common.ReadAndServeFile("assets/web/login.html", w)
		}

		d, err := ioutil.ReadFile("assets/config/users.json")
		if err != nil {

			return &common.HTTPError{
				Message:    fmt.Sprintf("error in reading users.json: %v", err),
				StatusCode: http.StatusInternalServerError,
			}
		}

		err = r.ParseForm()

		if err != nil {
			return &common.HTTPError{
				Message:    fmt.Sprintf("error in parsing form: %v", err),
				StatusCode: http.StatusBadRequest,
			}
		}

		username := r.Form.Get("username")
		password := r.Form.Get("password")
		if username == "" || password == "" {
			return &common.HTTPError{
				Message:    "username or password is empty",
				StatusCode: http.StatusBadRequest,
			}
		}

		var u map[string]string
		err = json.Unmarshal(d, &u) // Unmarshal into interface

		// Iterate through map until we find matching username
		for k, v := range u {
			if k == username && v == password {
				// Create a cookie here because the credentials are correct
				c, err := auth.CreateSession(&common.User{
					Username: k,
				})
				if err != nil {
					return &common.HTTPError{
						Message:    err.Error(),
						StatusCode: http.StatusInternalServerError,
					}
				}

				// r.AddCookie(c)
				w.Header().Add("Set-Cookie", c.String())
				// And now redirect the user to admin page
				http.Redirect(w, r, "/admin", http.StatusTemporaryRedirect)
				return nil
			}
		}

		return &common.HTTPError{
			Message:    "Invalid credentials!",
			StatusCode: http.StatusUnauthorized,
		}
	}
}

// Handles /, /feed and /json endpoints
func rootHandler() common.Handler {
	return func(rc *common.RouterContext, w http.ResponseWriter, r *http.Request) *common.HTTPError {

		var file string
		switch r.URL.Path {
		case "/rss":
			w.Header().Set("Content-Type", "application/rss+xml")
			file = "assets/web/feed.rss"
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			file = "assets/web/feed.json"
		case "/":
			w.Header().Set("Content-Type", "text/html")
			file = "assets/web/index.html"
		default:
			return &common.HTTPError{
				Message:    fmt.Sprintf("%s: Not Found", r.URL.Path),
				StatusCode: http.StatusNotFound,
			}
		}

		return common.ReadAndServeFile(file, w)
	}
}

func adminHandler() common.Handler {
	return func(rc *common.RouterContext, w http.ResponseWriter, r *http.Request) *common.HTTPError {
		return common.ReadAndServeFile("assets/web/admin.html", w)
	}
}

// Serve setup.html and config parameters
func serveSetup() common.Handler {
	return func(rc *common.RouterContext, w http.ResponseWriter, r *http.Request) *common.HTTPError {
		if r.Method == "GET" {
			return common.ReadAndServeFile("assets/web/setup.html", w)
		}
		r.ParseMultipartForm(32 << 20)

		// Parse form and convert to JSON
		cnf := NewConfig{
			strings.Join(r.Form["podcastname"], ""),  // Podcast name
			strings.Join(r.Form["podcasthost"], ""),  // Podcast host
			strings.Join(r.Form["podcastemail"], ""), // Podcast host email
			"", // Podcast image
			"", // Podcast location
			"", // Podcast location
		}

		b, err := json.Marshal(cnf)
		if err != nil {
			panic(err)
		}

		ioutil.WriteFile("assets/config/config.json", b, 0644)
		w.Write([]byte("Done"))
		return nil
	}
}
