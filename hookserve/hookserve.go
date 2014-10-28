package hookserve

import (
	"github.com/bmatsuo/go-jsontree"
	"io/ioutil"
	"net/http"
	"strconv"
)

// A list of valid github webhook IP addresses
// A request from an IP address not in this list will fail
var ValidIP []string = []string{
	"207.97.227.253",
	"50.57.128.197",
	"108.171.174.178",
	"50.57.231.61",
}

type Commit struct {
	Owner  string
	Repo   string
	Branch string
	Commit string
}

func (c *Commit) String() (output string) {
	output += "owner:  " + c.Owner + "\n"
	output += "repo:   " + c.Repo + "\n"
	output += "branch: " + c.Branch + "\n"
	output += "commit: " + c.Commit + "\n"
	return
}

type Server struct {
	Port   int
	Path   string
	Events chan Commit
}

func NewServer() *Server {
	return &Server{
		Port:   80,
		Path:   "/postreceive",
		Events: make(chan Commit, 10), // buffered to 10 items
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

	// Parse the request and build the Commit
	commit := Commit{}

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
		commit.Branch = rawRef[11:]
		commit.Repo, err = request.Get("repository").Get("name").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		commit.Commit, err = request.Get("head_commit").Get("id").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		commit.Owner, err = request.Get("repository").Get("owner").Get("name").String()
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
		commit.Repo, err = request.Get("pull_request").Get("head").Get("repo").Get("name").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		commit.Commit, err = request.Get("pull_request").Get("head").Get("sha").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		commit.Branch, err = request.Get("pull_request").Get("head").Get("ref").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		commit.Owner, err = request.Get("pull_request").Get("head").Get("repo").Get("owner").Get("login").String()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// We've built our Commit - put it into the channel and we're done
	go func() {
		s.Events <- commit
	}()

	w.Write([]byte(commit.String()))
	w.Write([]byte("\n\nOK"))
}
