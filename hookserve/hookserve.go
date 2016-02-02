package hookserve

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"github.com/bmatsuo/go-jsontree"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
)

var ErrInvalidEventFormat = errors.New("Unable to parse event string. Invalid Format.")

type Event struct {
	Owner      string // The username of the owner of the repository
	Repo       string // The name of the repository
	Branch     string // The branch the event took place on
	Commit     string // The head commit hash attached to the event
	Type       string // Can be either "pull_request" or "push"
	Action     string // For Pull Requests, contains "assigned", "unassigned", "labeled", "unlabeled", "opened", "closed", "reopened", or "synchronize".
	BaseOwner  string // For Pull Requests, contains the base owner
	BaseRepo   string // For Pull Requests, contains the base repo
	BaseBranch string // For Pull Requests, contains the base branch
}

// Create a new event from a string, the string format being the same as the one produced by event.String()
func NewEvent(e string) (*Event, error) {
	// Trim whitespace
	e = strings.Trim(e, "\n\t ")

	// Split into lines
	parts := strings.Split(e, "\n")

	// Sanity checking
	if len(parts) != 5 || len(parts) != 8 {
		return nil, ErrInvalidEventFormat
	}
	for _, item := range parts {
		if len(item) < 8 {
			return nil, ErrInvalidEventFormat
		}
	}

	// Fill in values for the event
	event := Event{}
	event.Type = parts[0][8:]
	event.Owner = parts[1][8:]
	event.Repo = parts[2][8:]
	event.Branch = parts[3][8:]
	event.Commit = parts[4][8:]

	// Fill in extra values if it's a pull_request
	if event.Type == "pull_request" {
		if len(parts) == 9 { // New format
			event.Action = parts[5][8:]
			event.BaseOwner = parts[6][8:]
			event.BaseRepo = parts[7][8:]
			event.BaseBranch = parts[8][8:]
		} else if len(parts) == 8 { // Old Format
			event.BaseOwner = parts[5][8:]
			event.BaseRepo = parts[6][8:]
			event.BaseBranch = parts[7][8:]
		} else {
			return nil, ErrInvalidEventFormat
		}
	}

	return &event, nil
}

func (e *Event) String() (output string) {
	output += "type:   " + e.Type + "\n"
	output += "owner:  " + e.Owner + "\n"
	output += "repo:   " + e.Repo + "\n"
	output += "branch: " + e.Branch + "\n"
	output += "commit: " + e.Commit + "\n"

	if e.Type == "pull_request" {
		output += "action: " + e.Action + "\n"
		output += "bowner: " + e.BaseOwner + "\n"
		output += "brepo:  " + e.BaseRepo + "\n"
		output += "bbranch:" + e.BaseBranch + "\n"
	}

	return
}

type Server struct {
	Port       int        // Port to listen on. Defaults to 80
	Path       string     // Path to receive on. Defaults to "/postreceive"
	Secret     string     // Option secret key for authenticating via HMAC
	IgnoreTags bool       // If set to false, also execute command if tag is pushed
	Events     chan Event // Channel of events. Read from this channel to get push events as they happen.
}

// Create a new server with sensible defaults.
// By default the Port is set to 80 and the Path is set to `/postreceive`
func NewServer() *Server {
	return &Server{
		Port:       80,
		Path:       "/postreceive",
		IgnoreTags: true,
		Events:     make(chan Event, 10), // buffered to 10 items
	}
}

// Spin up the server and listen for github webhook push events. The events will be passed to Server.Events channel.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(":"+strconv.Itoa(s.Port), s)
}

// Inside a go-routine, spin up the server and listen for github webhook push events. The events will be passed to Server.Events channel.
func (s *Server) GoListenAndServe() {
	go func() {
		err := s.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
}

// Checks if the given ref should be ignored
func (s *Server) ignoreRef(rawRef string) bool {
	if rawRef[:10] == "refs/tags/" && !s.IgnoreTags {
		return false
	}
	return rawRef[:11] != "refs/heads/"
}

// Satisfies the http.Handler interface.
// Instead of calling Server.ListenAndServe you can integrate hookserve.Server inside your own http server.
// If you are using hookserve.Server in his way Server.Path should be set to match your mux pattern and Server.Port will be ignored.
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

	// If we have a Secret set, we should check the MAC
	if s.Secret != "" {
		sig := req.Header.Get("X-Hub-Signature")

		if sig == "" {
			http.Error(w, "403 Forbidden - Missing X-Hub-Signature required for HMAC verification", http.StatusForbidden)
			return
		}

		mac := hmac.New(sha1.New, []byte(s.Secret))
		mac.Write(body)
		expectedMAC := mac.Sum(nil)
		expectedSig := "sha1=" + hex.EncodeToString(expectedMAC)
		if !hmac.Equal([]byte(expectedSig), []byte(sig)) {
			http.Error(w, "403 Forbidden - HMAC verification failed", http.StatusForbidden)
			return
		}
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
		if s.ignoreRef(rawRef) || request.Get("head_commit").IsNull() {
			return
		}

		// Fill in values
		event.Type = eventType
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
	} else if eventType == "pull_request" {
		event.Action, err = request.Get("action").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Fill in values
		event.Type = eventType
		event.Owner, err = request.Get("pull_request").Get("head").Get("repo").Get("owner").Get("login").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.Repo, err = request.Get("pull_request").Get("head").Get("repo").Get("name").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.Branch, err = request.Get("pull_request").Get("head").Get("ref").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.Commit, err = request.Get("pull_request").Get("head").Get("sha").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.BaseOwner, err = request.Get("pull_request").Get("base").Get("repo").Get("owner").Get("login").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.BaseRepo, err = request.Get("pull_request").Get("base").Get("repo").Get("name").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		event.BaseBranch, err = request.Get("pull_request").Get("base").Get("ref").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "Unknown Event Type "+eventType, http.StatusInternalServerError)
		return
	}

	// We've built our Event - put it into the channel and we're done
	go func() {
		s.Events <- event
	}()

	w.Write([]byte(event.String()))
}
