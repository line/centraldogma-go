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
Package centraldogma provides a Go client for accessing Central Dogma.
Visit https://line.github.io/centraldogma/ to learn more about Central Dogma.

Usage:

	import "go.linecorp.com/centraldogma"

Create a client with the username and password, then use the client to access the
Central Dogma HTTP APIs. For example:

	username := "foo"
	password := "bar"
	client, err := centraldogma.NewClientWithToken("https://localhost:443", "myToken", nil)

	projects, res, err := client.ListProjects(context.Background())

Note that all of the APIs are using the https://godoc.org/context which can pass
cancellation and deadlines for handling a request.
*/
package centraldogma

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	metrics "github.com/armon/go-metrics"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/http2"
	"golang.org/x/oauth2"
)

var log = logrus.New()

const (
	defaultScheme     = "https"
	defaultHostName   = "localhost"
	defaultBaseURL    = defaultScheme + "://" + defaultHostName + "/"
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

	// metrics
	metricCollector *metrics.Metrics
}

type service struct {
	client *Client
}

// NewClientWithToken returns a Central Dogma client which communicates the server at baseURL, using the specified
// token and transport. If transport is nil, http2.Transport is used by default.
func NewClientWithToken(baseURL, token string, transport http.RoundTripper) (*Client, error) {
	normalizedURL, err := normalizeURL(baseURL)
	if err != nil {
		return nil, err
	}

	client, err := newOAuth2HTTP2Client(normalizedURL.String(), token, transport)
	if err != nil {
		return nil, err
	}

	return newClientWithHTTPClient(normalizedURL, client)
}

// DefaultOAuth2Transport returns an oauth2.Transport which internally uses the specified transport and attaches
// the specified token to every request using the authorization header. If the transport is a type of oauth2.Transport,
// it will throw an error.
func DefaultOAuth2Transport(baseURL, token string, transport http.RoundTripper) (*oauth2.Transport, error) {
	if len(token) == 0 {
		return nil, ErrTokenEmpty
	}
	if transport == nil {
		return nil, ErrTransportMustBeSet
	}

	_, ok := transport.(*oauth2.Transport)
	if ok {
		return nil, ErrTransportMustNotBeOAuth2
	}

	normalizedURL, err := normalizeURL(baseURL)
	if err != nil {
		return nil, err
	}

	config := oauth2.Config{Endpoint: oauth2.Endpoint{TokenURL: normalizedURL.String() + pathLogin}}
	tokenSource := config.TokenSource(context.Background(), &oauth2.Token{AccessToken: token})
	return &oauth2.Transport{
		Base:   transport,
		Source: oauth2.ReuseTokenSource(nil, tokenSource),
	}, nil
}

// DefaultHTTP2Transport returns a http2.Transport which could be used on cleartext or encrypted connection depending
// on the scheme of the baseURL.
func DefaultHTTP2Transport(baseURL string) (*http2.Transport, error) {
	normalizedURL, err := normalizeURL(baseURL)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(normalizedURL.String(), "http://") { // H2C
		return &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		}, nil
	}
	return &http2.Transport{}, nil // H2
}

func newOAuth2HTTP2Client(normalizedURL, token string, transport http.RoundTripper) (c *http.Client, err error) {
	if transport == nil {
		transport, err = DefaultHTTP2Transport(normalizedURL)
		if err != nil {
			return nil, err
		}
	}

	_, ok := transport.(*oauth2.Transport)
	if !ok {
		transport, err = DefaultOAuth2Transport(normalizedURL, token, transport)
		if err != nil {
			return nil, err
		}
	}
	return &http.Client{Transport: transport}, nil
}

// newClientWithHTTPClient returns a Central Dogma client with the specified baseURL and client.
// The client should perform the authentication.
func newClientWithHTTPClient(baseURL *url.URL, client *http.Client) (*Client, error) {
	c := &Client{
		client:  client,
		baseURL: baseURL,
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
	// build relative url
	u, err := url.Parse(pathSecurityEnabled)
	if err != nil {
		return false, err
	}

	req, err := c.newRequest(http.MethodGet, u, nil)
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

func (c *Client) newRequest(method string, url *url.URL, body interface{}) (*http.Request, error) {
	// resolves a URI reference to an absolute URI from base URI
	u := c.baseURL.ResolveReference(url)

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

func drainupAndCloseResponseBody(body io.ReadCloser) {
	if body != nil {
		// drain up and close the body to reuse connection
		// see also:
		// - https://github.com/google/go-github/pull/317
		// - https://forum.golangbridge.org/t/do-i-need-to-read-the-body-before-close-it/5594/4
		_, _ = io.Copy(ioutil.Discard, body)

		// close body
		body.Close()
	}
}

func (c *Client) do(ctx context.Context,
	req *http.Request, resContent interface{}, watchRequest bool) (statusCode int, err error) {
	req = req.WithContext(ctx)

	// prepare metrics
	var metricLabels []metrics.Label
	if c.metricCollector != nil {
		metricLabels = []metrics.Label{
			{Name: "method", Value: req.Method},
			{Name: "host", Value: req.URL.Host},              // included port
			{Name: "path", Value: req.URL.EscapedPath()},     // escaped path
			{Name: "query", Value: req.URL.Query().Encode()}, // encoded query
		}
	}

	// mark the time point when request begins
	startAt := time.Now()

	// make request
	res, err := c.client.Do(req)

	// get response status code
	if err == nil {
		statusCode = res.StatusCode
	} else {
		statusCode = UnknownHttpStatusCode
	}

	// report duration metric (even if error happened)
	if c.metricCollector != nil {
		metricLabels = append(metricLabels, metrics.Label{Name: "statusCode", Value: strconv.Itoa(statusCode)})
		c.metricCollector.MeasureSinceWithLabels([]string{"requestDuration"}, startAt, metricLabels)
	}

	// check request error
	if err != nil {
		if c.metricCollector != nil {
			c.metricCollector.IncrCounter([]string{"totalRequestFail"}, 1)
		}
		return
	}

	// handling status code
	startAt = time.Now()
	if !watchRequest || statusCode != http.StatusNotModified {
		if statusCode < 200 || statusCode >= 300 {
			errorMessage := &errorMessage{}

			err = json.NewDecoder(res.Body).Decode(errorMessage)
			if err != nil {
				err = fmt.Errorf("status: %v", statusCode)
			} else {
				err = fmt.Errorf("%s (status: %v)", errorMessage.Message, statusCode)
			}
		} else if resContent != nil {
			err = json.NewDecoder(res.Body).Decode(resContent)
			if err == io.EOF { // empty response body
				err = nil
			}
		}
	}

	// report metric
	if c.metricCollector != nil {
		c.metricCollector.MeasureSinceWithLabels([]string{"parseDuration"}, startAt, metricLabels)
	}

	// never forget to drain up and close before returning
	drainupAndCloseResponseBody(res.Body)

	return
}

// CreateProject creates a project.
func (c *Client) CreateProject(ctx context.Context, name string) (pro *Project, httpStatusCode int, err error) {
	return c.project.create(ctx, name)
}

// RemoveProject removes a project. A removed project can be unremoved using UnremoveProject.
func (c *Client) RemoveProject(ctx context.Context, name string) (httpStatusCode int, err error) {
	return c.project.remove(ctx, name)
}

// PurgeProject purges a project which was removed before.
func (c *Client) PurgeProject(ctx context.Context, name string) (httpStatusCode int, err error) {
	return c.project.purge(ctx, name)
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

// PurgeRepository purges a repository which was removed before.
func (c *Client) PurgeRepository(ctx context.Context, projectName, repoName string) (httpStatusCode int, err error) {
	return c.repository.purge(ctx, projectName, repoName)
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
	ctx context.Context, projectName, repoName, revision string) (normalizedRev int64, httpStatusCode int, err error) {
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
	_ = w.Watch(func(value WatchResult) {
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

// SetMetricCollector sets metric collector for the client.
// For example, with Prometheus:
//     config := centraldogma.DefaultMetricCollectorConfig("client_name")
//     metricCollector := centraldogma.GlobalPrometheusMetricCollector(config)
//     client.SetMetricCollector(metricCollector)
//
// Or Statsd:
//     config := centraldogma.DefaultMetricCollectorConfig("client_name")
//     metricCollector, err := centraldogma.StatsdMetricCollector(config, "127.0.0.1:8125")
//     if err != nil {
//         panic(err)
//     }
//     client.SetMetricCollector(metricCollector)
func (c *Client) SetMetricCollector(m *metrics.Metrics) {
	c.metricCollector = m
}
