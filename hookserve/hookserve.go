package hookserve

import (
	"github.com/bmatsuo/go-jsontree"
	"io/ioutil"
	"net/http"
	"strconv"
)

type Event struct {
	Owner  string
	Repo   string
	Branch string
	Commit string
}

func (e *Event) String() (output string) {
	output += "owner:  " + e.Owner + "\n"
	output += "repo:   " + e.Repo + "\n"
	output += "branch: " + e.Branch + "\n"
	output += "commit: " + e.Commit + "\n"
	return
}

type Server struct {
	Port   int        // Port to listen on. Defaults to 80
	Path   string     // Path to receive on. Defaults to postreceive
	Secret string     // Option secret key for authenticating via HMAC
	Events chan Event // Channel of events. Read from this channel to get push events as they happen.
}

func NewServer() *Server {
	return &Server{
		Port:   80,
		Path:   "/postreceive",
		Events: make(chan Event, 10), // buffered to 10 items
	}
}

func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(":"+strconv.Itoa(s.Port), s)
}

func (s *Server) GoListenAndServe() {
	go func() {
		err := s.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	if req.Method != "POST" {
		http.Error(w, "405 Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if req.URL.Path != s.Path {
		http.Error(w, "404 Not found", http.StatusNotFound)
		return
	}

	eventType := req.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "400 Bad Request - Missing X-GitHub-Event Header", http.StatusBadRequest)
		return
	}
	if eventType != "push" && eventType != "pull_request" {
		http.Error(w, "400 Bad Request - Unknown Event Type "+eventType, http.StatusBadRequest)
		return
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	request := jsontree.New()
	err = request.UnmarshalJSON(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the request and build the Event
	event := Event{}

	if eventType == "push" {
		rawRef, err := request.Get("ref").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// If the ref is not a branch, we don't care about it
		if rawRef[:11] != "refs/heads/" || request.Get("head_commit").IsNull() {
			return
		}

		// Fill in values
		event.Branch = rawRef[11:]
		event.Repo, err = request.Get("repository").Get("name").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.Commit, err = request.Get("head_commit").Get("id").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.Owner, err = request.Get("repository").Get("owner").Get("name").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if eventType == "pull_request" {
		action, err := request.Get("action").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// If the action is not to open or to synchronize we don't care about it
		if action != "synchronize" && action != "opened" {
			return
		}

		// Fill in values
		event.Repo, err = request.Get("pull_request").Get("head").Get("repo").Get("name").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.Commit, err = request.Get("pull_request").Get("head").Get("sha").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.Branch, err = request.Get("pull_request").Get("head").Get("ref").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.Owner, err = request.Get("pull_request").Get("head").Get("repo").Get("owner").Get("login").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// We've built our Event - put it into the channel and we're done
	go func() {
		s.Events <- event
	}()

	w.Write([]byte(event.String()))
}
