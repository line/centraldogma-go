// Copyright 2018 LINE Corporation
//
// LINE Corporation licenses this file to you under the Apache License,
// version 2.0 (the "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at:
//
//   https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

/*
Package 'centraldogma' provides a Go client for accessing Central Dogma.
Visit https://line.github.io/centraldogma/ to learn more about Central Dogma.

Usage:

	import "go.linecorp.com/centraldogma"

Create a client with the username and password, then use the client to access the
Central Dogma HTTP APIs. For example:

	username := "foo"
	password := "bar"
	client, err := centraldogma.NewClient("http://localhost:36462", username, password)

	projects, res, err := client.ListProjects(context.Background())

Note that all of the APIs are using the https://godoc.org/context which can pass
cancellation and deadlines for handling a request.
*/
package centraldogma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

var log = logrus.New()

const (
	defaultScheme     = "http"
	defaultHostName   = "localhost"
	defaultBaseURL    = defaultScheme + "://" + defaultHostName + ":36462/"
	defaultPathPrefix = "api/v1/"

	pathSecurityEnabled = "security_enabled"
	pathLogin           = defaultPathPrefix + "login"
)

// A Client communicates with the Central Dogma server API.
type Client struct {
	client *http.Client // HTTP client which sends the request.

	baseURL *url.URL // Base URL for API requests.

	// Services are used to communicate for the different parts of the Central Dogma server API.
	project    *projectService
	repository *repositoryService
	content    *contentService
	watch      *watchService
}

type service struct {
	client *Client
}

// NewClient returns a Central Dogma client with the specified baseURL, username and password.
func NewClient(baseURL, username, password string) (*Client, error) {
	normalizedURL, err := normalizeURL(baseURL)
	if err != nil {
		return nil, err
	}

	config := oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: normalizedURL.String() + pathLogin}}
	token, err := config.PasswordCredentialsToken(context.Background(), username, password)
	if err != nil {
		return nil, err
	}

	return newClientWithHTTPClient(normalizedURL.String(), config.Client(context.Background(), token))
}

// NewClientWithToken returns a Central Dogma client with the specified baseURL and token.
func NewClientWithToken(baseURL, token string) (*Client, error) {
	normalizedURL, err := normalizeURL(baseURL)
	if err != nil {
		return nil, err
	}

	config := oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: normalizedURL.String() + pathLogin}}
	oauthToken := &oauth2.Token{AccessToken: token}

	return newClientWithHTTPClient(normalizedURL.String(), config.Client(context.Background(), oauthToken))
}

// newClientWithHTTPClient returns a Central Dogma client with the specified baseURL and client.
// The client should perform the authentication.
func newClientWithHTTPClient(baseURL string, client *http.Client) (*Client, error) {
	normalizedURL, err := normalizeURL(baseURL)
	if err != nil {
		return nil, err
	}

	c := &Client{
		client:  client,
		baseURL: normalizedURL,
	}
	service := &service{client: c}

	c.project = (*projectService)(service)
	c.repository = (*repositoryService)(service)
	c.content = (*contentService)(service)
	c.watch = (*watchService)(service)
	return c, nil
}

func normalizeURL(baseURL string) (*url.URL, error) {
	if len(baseURL) == 0 {
		return url.Parse(defaultBaseURL)
	}

	if !strings.HasPrefix(baseURL, "http") {
		// Prepend the defaultScheme when there is no specified scheme so parse the url properly
		// in case of "hostname:port".
		baseURL = defaultScheme + "://" + baseURL
	}

	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	return url.Parse(baseURL)
}

// SecurityEnabled returns whether the security of the server is enabled or not.
func (c *Client) SecurityEnabled() (bool, error) {
	req, err := c.newRequest(http.MethodGet, pathSecurityEnabled, nil)
	if err != nil {
		return false, err
	}

	res, err := c.client.Do(req)
	if err != nil {
		return false, err
	}

	if res.StatusCode != http.StatusOK {
		// The security is not enabled.
		return false, nil
	}
	return true, nil
}

func (c *Client) newRequest(method, urlStr string, body interface{}) (*http.Request, error) {
	u, err := c.baseURL.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	var buf io.ReadWriter
	if body != nil {
		if str, ok := body.(string); ok {
			buf = bytes.NewBufferString(str)
		} else {
			buf = new(bytes.Buffer)
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(true)
			err := enc.Encode(body)
			if err != nil {
				return nil, err
			}
		}
	}

	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	if auth := req.Header.Get("Authorization"); len(auth) == 0 {
		req.Header.Set("Authorization", "Bearer anonymous")
	}

	if body != nil {
		if method == http.MethodPatch {
			req.Header.Set("Content-Type", "application/json-patch+json")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	return req, nil
}

type errorMessage struct {
	Message string `json:"message"`
}

func (c *Client) do(ctx context.Context, req *http.Request, resContent interface{}) (*http.Response, error) {
	req = req.WithContext(ctx)

	res, err := c.client.Do(req)
	if err != nil {
		return &http.Response{StatusCode: -1}, err
	}
	defer func() {
		// drain up 512 bytes and close the body to reuse connection
		// see also:
		// - https://github.com/google/go-github/pull/317
		// - https://forum.golangbridge.org/t/do-i-need-to-read-the-body-before-close-it/5594/4
		io.CopyN(ioutil.Discard, res.Body, 512)

		res.Body.Close()
	}()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		errorMessage := &errorMessage{}

		err = json.NewDecoder(res.Body).Decode(errorMessage)
		if err != nil {
			err = fmt.Errorf("status: %v", res.StatusCode)
		} else {
			err = fmt.Errorf("%s (status: %v)", errorMessage.Message, res.StatusCode)
		}

		return res, err
	}

	if resContent != nil {
		err = json.NewDecoder(res.Body).Decode(resContent)
		if err == io.EOF { // empty response body
			err = nil
		}
	}
	return res, err
}

// CreateProject creates a project.
func (c *Client) CreateProject(ctx context.Context, name string) (pro *Project, httpStatusCode int, err error) {
	return c.project.create(ctx, name)
}

// RemoveProject removes a project. A removed project can be unremoved using UnremoveProject.
func (c *Client) RemoveProject(ctx context.Context, name string) (httpStatusCode int, err error) {
	return c.project.remove(ctx, name)
}

// UnremoveProject unremoves a removed project.
func (c *Client) UnremoveProject(ctx context.Context, name string) (pro *Project, httpStatusCode int, err error) {
	return c.project.unremove(ctx, name)
}

// ListProjects returns the list of projects.
func (c *Client) ListProjects(ctx context.Context) (pros []*Project, httpStatusCode int, err error) {
	return c.project.list(ctx)
}

// ListRemovedProjects returns the list of removed projects.
func (c *Client) ListRemovedProjects(ctx context.Context) (removedPros []*Project, httpStatusCode int, err error) {
	return c.project.listRemoved(ctx)
}

// CreateRepository creates a repository.
func (c *Client) CreateRepository(
	ctx context.Context, projectName, repoName string) (repo *Repository, httpStatusCode int, err error) {
	return c.repository.create(ctx, projectName, repoName)
}

// RemoveRepository removes a repository. A removed repository can be unremoved using UnremoveRepository.
func (c *Client) RemoveRepository(ctx context.Context, projectName, repoName string) (httpStatusCode int, err error) {
	return c.repository.remove(ctx, projectName, repoName)
}

// UnremoveRepository unremoves a repository.
func (c *Client) UnremoveRepository(
	ctx context.Context, projectName, repoName string) (repo *Repository, httpStatusCode int, err error) {
	return c.repository.unremove(ctx, projectName, repoName)
}

// ListRepositories returns the list of repositories.
func (c *Client) ListRepositories(
	ctx context.Context, projectName string) (repos []*Repository, httpStatusCode int, err error) {
	return c.repository.list(ctx, projectName)
}

// ListRemovedRepositories returns the list of the removed repositories which can be unremoved using
// UnremoveRepository.
func (c *Client) ListRemovedRepositories(
	ctx context.Context, projectName string) (removedRepos []*Repository, httpStatusCode int, err error) {
	return c.repository.listRemoved(ctx, projectName)
}

// NormalizeRevision converts the relative revision number to the absolute revision number(e.g. -1 -> 3).
func (c *Client) NormalizeRevision(
	ctx context.Context, projectName, repoName, revision string) (normalizedRev int, httpStatusCode int, err error) {
	return c.repository.normalizeRevision(ctx, projectName, repoName, revision)
}

// ListFiles returns the list of files that match the given path pattern. A path pattern is a variant of glob:
//
//     - "/**": find all files recursively
//     - "*.json": find all JSON files recursively
//     - "/foo/*.json": find all JSON files under the directory /foo
//     - "/&#42;/foo.txt": find all files named foo.txt at the second depth level
//     - "*.json,/bar/*.txt": use comma to match any patterns
//
func (c *Client) ListFiles(ctx context.Context,
	projectName, repoName, revision, pathPattern string) (entries []*Entry, httpStatusCode int, err error) {
	return c.content.listFiles(ctx, projectName, repoName, revision, pathPattern)
}

// GetFile returns the file at the specified revision and path with the specified Query.
func (c *Client) GetFile(
	ctx context.Context, projectName, repoName, revision string, query *Query) (entry *Entry,
	httpStatusCode int, err error) {
	return c.content.getFile(ctx, projectName, repoName, revision, query)
}

// GetFiles returns the files that match the given path pattern. A path pattern is a variant of glob:
//
//     - "/**": find all files recursively
//     - "*.json": find all JSON files recursively
//     - "/foo/*.json": find all JSON files under the directory /foo
//     - "/&#42;/foo.txt": find all files named foo.txt at the second depth level
//     - "*.json,/bar/*.txt": use comma to match any patterns
//
func (c *Client) GetFiles(ctx context.Context,
	projectName, repoName, revision, pathPattern string) (entries []*Entry, httpStatusCode int, err error) {
	return c.content.getFiles(ctx, projectName, repoName, revision, pathPattern)
}

// GetHistory returns the history of the files that match the given path pattern. A path pattern is
// a variant of glob:
//
//     - "/**": find all files recursively
//     - "*.json": find all JSON files recursively
//     - "/foo/*.json": find all JSON files under the directory /foo
//     - "/&#42;/foo.txt": find all files named foo.txt at the second depth level
//     - "*.json,/bar/*.txt": use comma to match any patterns
//
// If the from and to are not specified, this will return the history from the init to the latest revision.
func (c *Client) GetHistory(ctx context.Context,
	projectName, repoName, from, to, pathPattern string, maxCommits int) (commits []*Commit,
	httpStatusCode int, err error) {
	return c.content.getHistory(ctx, projectName, repoName, from, to, pathPattern, maxCommits)
}

// GetDiff returns the diff of a file between two revisions. If the from and to are not specified, this will
// return the diff from the init to the latest revision.
func (c *Client) GetDiff(ctx context.Context,
	projectName, repoName, from, to string, query *Query) (change *Change, httpStatusCode int, err error) {
	return c.content.getDiff(ctx, projectName, repoName, from, to, query)
}

// GetDiffs returns the diffs of the files that match the given path pattern. A path pattern is
// a variant of glob:
//
//     - "/**": find all files recursively
//     - "*.json": find all JSON files recursively
//     - "/foo/*.json": find all JSON files under the directory /foo
//     - "/&#42;/foo.txt": find all files named foo.txt at the second depth level
//     - "*.json,/bar/*.txt": use comma to match any patterns
//
// If the from and to are not specified, this will return the diffs from the init to the latest revision.
func (c *Client) GetDiffs(ctx context.Context,
	projectName, repoName, from, to, pathPattern string) (changes []*Change, httpStatusCode int, err error) {
	return c.content.getDiffs(ctx, projectName, repoName, from, to, pathPattern)
}

// Push pushes the specified changes to the repository.
func (c *Client) Push(ctx context.Context, projectName, repoName, baseRevision string,
	commitMessage *CommitMessage, changes []*Change) (result *PushResult, httpStatusCode int, err error) {
	return c.content.push(ctx, projectName, repoName, baseRevision, commitMessage, changes)
}

func (c *Client) watchWithWatcher(w *Watcher) (result <-chan WatchResult, closer func()) {
	// setup watching channel
	ch := make(chan WatchResult, DefaultChannelBuffer)
	result = ch
	w.Watch(func(value WatchResult) {
		ch <- value
	})

	// setup closer
	closer = func() {
		w.Close()
	}

	// start watching
	w.start()
	return
}

// WatchFile watches on file changes. The watched result will be returned
// through the returned channel. The API also provides a manual closer to stop watching
// and release underlying resources.
// In short, watching will be stopped in case either context is cancelled or closer is
// called.
// Manually closing returned channel is unsafe and may cause sending on closed channel error.
// Usage:
//
//    query := &Query{Path: "/a.json", Type: Identity}
//    ctx := context.Background()
//    changes, closer, err := client.WatchFile(ctx, "foo", "bar", query, 2 * time.Minute)
//    if err != nil {
//		 panic(err)
//    }
//    defer closer() // stop watching and release underlying resources.
//
//    /* close(changes) */ // manually closing is unsafe, don't do this.
//
//    for {
//        select {
//          case <-ctx.Done():
//             ...
//
//          case change := <-changes:
//             // got change
//             json.Unmarshal(change.Entry.Content, &expect)
//             ...
//        }
//    }
func (c *Client) WatchFile(
	ctx context.Context,
	projectName, repoName string, query *Query,
	timeout time.Duration,
) (result <-chan WatchResult, closer func(), err error) {

	var w *Watcher

	// initialize watcher
	w, err = c.watch.fileWatcherWithTimeout(ctx, projectName, repoName, query, timeout)
	if err != nil {
		return
	}

	result, closer = c.watchWithWatcher(w)
	return
}

// WatchRepository watches on repository changes. The watched result will be returned
// through the returned channel. The API also provides a manual closer to stop watching
// and release underlying resources.
// In short, watching will be stopped in case either context is cancelled or closer is
// called.
// Manually closing returned channel is unsafe and may cause sending on closed channel error.
// Usage:
//
//    query := &Query{Path: "/a.json", Type: Identity}
//    ctx := context.Background()
//    changes, closer, err := client.WatchRepository(ctx, "foo", "bar", "/*.json", 2 * time.Minute)
//    if err != nil {
//		 panic(err)
//    }
//    defer closer() // stop watching and release underlying resources.
//
//    /* close(changes) */ // manually closing is unsafe, don't do this.
//
//    for {
//        select {
//          case <-ctx.Done():
//             ...
//
//          case change := <-changes:
//             // got change
//             json.Unmarshal(change.Entry.Content, &expect)
//             ...
//        }
//    }
func (c *Client) WatchRepository(
	ctx context.Context,
	projectName, repoName, pathPattern string,
	timeout time.Duration,
) (result <-chan WatchResult, closer func(), err error) {

	var w *Watcher

	// initialize watcher
	w, err = c.watch.repoWatcherWithTimeout(ctx, projectName, repoName, pathPattern, timeout)
	if err != nil {
		return
	}

	result, closer = c.watchWithWatcher(w)
	return
}

// FileWatcher returns a Watcher which notifies its listeners when the result of the given Query becomes
// available or changes. For example:
//
//    query := &Query{Path: "/a.json", Type: Identity}
//    watcher := client.FileWatcher("foo", "bar", query)
//
//    myCh := make(chan interface{})
//    watcher.Watch(func(revision int, value interface{}) {
//        myCh <- value
//    })
//    myValue := <-myCh
func (c *Client) FileWatcher(projectName, repoName string, query *Query) (*Watcher, error) {
	fw, err := c.watch.fileWatcher(context.Background(), projectName, repoName, query)
	if err != nil {
		return nil, err
	}
	fw.start()
	return fw, nil
}

// RepoWatcher returns a Watcher which notifies its listeners when the repository that matched the given
// pathPattern becomes available or changes. For example:
//
//    watcher := client.RepoWatcher("foo", "bar", "/*.json")
//
//    myCh := make(chan interface{})
//    watcher.Watch(func(revision int, value interface{}) {
//        myCh <- value
//    })
//    myValue := <-myCh
func (c *Client) RepoWatcher(projectName, repoName, pathPattern string) (*Watcher, error) {
	rw, err := c.watch.repoWatcher(context.Background(), projectName, repoName, pathPattern)
	if err != nil {
		return nil, err
	}
	rw.start()
	return rw, nil
}
