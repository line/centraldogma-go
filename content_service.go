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

package centraldogma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type contentService service

// Query specifies a query on a file.
type Query struct {
	Path string
	// QueryType can be "identity" or "json_path". "identity" is used to retrieve the content as it is.
	// "json_path" applies a series of JSON path to the content.
	// See https://github.com/json-path/JsonPath/blob/master/README.md
	Type        QueryType
	Expressions []string
}

type QueryType int

const (
	Identity QueryType = iota + 1
	JSONPath
)

// Entry represents an entry in the repository.
type Entry struct {
	Path       string       `json:"path"`
	Type       EntryType    `json:"type"` // can be JSON, TEXT or DIRECTORY
	Content    EntryContent `json:"content,omitempty"`
	Revision   string       `json:"revision,omitempty"`
	URL        string       `json:"url,omitempty"`
	ModifiedAt string       `json:"modifiedAt,omitempty"`
}

func (c *Entry) MarshalJSON() ([]byte, error) {
	type Alias Entry
	return json.Marshal(&struct {
		Type string `json:"type"`
		*Alias
	}{
		Type:  c.Type.String(),
		Alias: (*Alias)(c),
	})
}

func (c *Entry) UnmarshalJSON(b []byte) error {
	type Alias Entry
	auxiliary := &struct {
		Type string `json:"type"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(b, &auxiliary); err != nil {
		return err
	}
	c.Type = entryTypeMap[auxiliary.Type]
	return nil
}

// EntryContent represents the content of an entry
type EntryContent []byte

func (e *EntryContent) UnmarshalJSON(b []byte) error {
	if n := len(b); n >= 2 && b[0] == 34 && b[n-1] == 34 { // string
		var dst string
		if err := json.Unmarshal(b, &dst); err != nil {
			return err
		}
		*e = []byte(dst)
	} else {
		*e = b
	}
	return nil
}

// PushResult represents a result of push in the repository.
type PushResult struct {
	Revision int    `json:"revision"`
	PushedAt string `json:"pushedAt"`
}

// Commit represents a commit in the repository.
type Commit struct {
	Revision      int           `json:"revision"`
	Author        Author        `json:"author,omitempty"`
	CommitMessage CommitMessage `json:"commitMessage,omitempty"`
	PushedAt      string        `json:"pushedAt,omitempty"`
}

// CommitMessages represents a commit message in the repository.
type CommitMessage struct {
	Summary string `json:"summary"`
	Detail  string `json:"detail,omitempty"`
	Markup  string `json:"markup,omitempty"`
}

// Change represents a change to commit in the repository.
type Change struct {
	Path    string      `json:"path"`
	Type    ChangeType  `json:"type"`
	Content interface{} `json:"content,omitempty"`
}

func (c *Change) MarshalJSON() ([]byte, error) {
	type Alias Change
	return json.Marshal(&struct {
		Type string `json:"type"`
		*Alias
	}{
		Type:  c.Type.String(),
		Alias: (*Alias)(c),
	})
}

func (c *Change) UnmarshalJSON(b []byte) error {
	type Alias Change
	auxiliary := &struct {
		Type string `json:"type"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if err := json.Unmarshal(b, &auxiliary); err != nil {
		return err
	}
	c.Type = changeTypeMap[auxiliary.Type]
	return nil
}

func (con *contentService) listFiles(ctx context.Context,
	projectName, repoName, revision, pathPattern string) ([]*Entry, int, error) {
	if len(pathPattern) != 0 && !strings.HasPrefix(pathPattern, "/") {
		// Normalize the pathPattern when it does not start with "/" so that the pathPattern fits into the url.
		pathPattern = "/**/" + pathPattern
	}

	u := fmt.Sprintf("%vprojects/%v/repos/%v/list%v", defaultPathPrefix, projectName, repoName, pathPattern)

	if len(revision) != 0 {
		v := &url.Values{}
		v.Set("revision", revision)
		u += encodeValues(v)
	}
	req, err := con.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	var entries []*Entry
	statusCode, err := con.client.do(ctx, req, &entries)
	if err != nil {
		return nil, statusCode, err
	}
	return entries, statusCode, nil
}

func encodeValues(v *url.Values) string {
	if encoded := v.Encode(); len(encoded) != 0 {
		return "?" + encoded
	}
	return ""
}

func (con *contentService) getFile(
	ctx context.Context, projectName, repoName, revision string, query *Query) (*Entry, int, error) {
	if query == nil {
		return nil, UnknownHttpStatusCode, errors.New("query should not be nil")
	}

	path := query.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	u := fmt.Sprintf("%vprojects/%v/repos/%v/contents%v", defaultPathPrefix, projectName, repoName, path)
	v := &url.Values{}
	if err := getFileURLValues(v, revision, path, query); err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	u += encodeValues(v)

	req, err := con.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	entry := new(Entry)
	statusCode, err := con.client.do(ctx, req, entry)
	if err != nil {
		return nil, statusCode, err
	}

	return entry, statusCode, nil
}

// getFileURLValues currently only supports JSON path.
func getFileURLValues(v *url.Values, revision, path string, query *Query) error {
	if query.Type == JSONPath {
		if err := setJSONPaths(v, path, query.Expressions); err != nil {
			return err
		}
	}

	if len(revision) != 0 {
		// have both of the jsonPath and the revision
		v.Set("revision", revision)
	}
	return nil
}

func setJSONPaths(v *url.Values, path string, jsonPaths []string) error {
	if !strings.HasSuffix(strings.ToLower(path), "json") {
		return fmt.Errorf("the extension of the file should be .json (path: %v)", path)
	}
	for _, jsonPath := range jsonPaths {
		v.Add("jsonpath", jsonPath)
	}
	return nil
}

func (con *contentService) getFiles(ctx context.Context,
	projectName, repoName, revision, pathPattern string) ([]*Entry, int, error) {
	if len(pathPattern) != 0 && !strings.HasPrefix(pathPattern, "/") {
		// Normalize the pathPattern when it does not start with "/" so that the pathPattern fits into the url.
		pathPattern = "/**/" + pathPattern
	}
	u := fmt.Sprintf("%vprojects/%v/repos/%v/contents%v", defaultPathPrefix, projectName, repoName, pathPattern)

	if len(revision) != 0 {
		v := &url.Values{}
		v.Set("revision", revision)
		u += encodeValues(v)
	}

	req, err := con.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	var entries []*Entry
	statusCode, err := con.client.do(ctx, req, &entries)
	if err != nil {
		return nil, statusCode, err
	}
	return entries, statusCode, nil
}

func (con *contentService) getHistory(ctx context.Context,
	projectName, repoName, from, to, pathPattern string, maxCommits int) ([]*Commit, int, error) {
	u := fmt.Sprintf("%vprojects/%v/repos/%v/commits/%v", defaultPathPrefix, projectName, repoName, from)

	v := &url.Values{}
	if len(pathPattern) != 0 {
		v.Set("path", pathPattern)
	}
	if len(to) != 0 {
		v.Set("to", to)
	}
	if maxCommits != 0 {
		v.Set("maxCommits", strconv.Itoa(maxCommits))
	}
	u += encodeValues(v)

	req, err := con.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	var commits []*Commit
	statusCode, err := con.client.do(ctx, req, &commits)
	if err != nil {
		return nil, statusCode, err
	}
	return commits, statusCode, nil
}

func (con *contentService) getDiff(ctx context.Context,
	projectName, repoName, from, to string, query *Query) (*Change, int, error) {
	if query == nil {
		return nil, UnknownHttpStatusCode, errors.New("query should not be nil")
	}

	path := query.Path
	if len(path) == 0 {
		return nil, UnknownHttpStatusCode, errors.New("the path should not be empty")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	u := fmt.Sprintf("%vprojects/%v/repos/%v/compare", defaultPathPrefix, projectName, repoName)
	v := &url.Values{}
	v.Set("path", path)
	if query != nil && query.Type == JSONPath {
		if err := setJSONPaths(v, path, query.Expressions); err != nil {
			return nil, UnknownHttpStatusCode, err
		}
	}
	setFromTo(v, from, to)
	u += encodeValues(v)

	req, err := con.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	change := new(Change)
	statusCode, err := con.client.do(ctx, req, change)
	if err != nil {
		return nil, statusCode, err
	}

	return change, statusCode, nil
}

func setFromTo(v *url.Values, from, to string) {
	if len(from) != 0 {
		v.Set("from", from)
	}

	if len(to) != 0 {
		v.Set("to", to)
	}
}

func (con *contentService) getDiffs(ctx context.Context,
	projectName, repoName, from, to, pathPattern string) ([]*Change, int, error) {
	u := fmt.Sprintf("%vprojects/%v/repos/%v/compare", defaultPathPrefix, projectName, repoName)
	v := &url.Values{}

	if len(pathPattern) == 0 {
		pathPattern = "/**"
	}
	v.Set("pathPattern", pathPattern)
	setFromTo(v, from, to)
	u += encodeValues(v)

	req, err := con.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	var changes []*Change
	statusCode, err := con.client.do(ctx, req, &changes)
	if err != nil {
		return nil, statusCode, err
	}
	return changes, statusCode, nil
}

type push struct {
	CommitMessage *CommitMessage `json:"commitMessage"`
	Changes       []*Change      `json:"changes"`
}

func (con *contentService) push(ctx context.Context, projectName, repoName, baseRevision string,
	commitMessage *CommitMessage, changes []*Change) (*PushResult, int, error) {
	if len(commitMessage.Summary) == 0 {
		return nil, UnknownHttpStatusCode, fmt.Errorf(
			"summary of commitMessage cannot be empty. commitMessage: %+v", commitMessage)
	}

	if len(changes) == 0 {
		return nil, UnknownHttpStatusCode, errors.New("no changes to commit")
	}

	u := fmt.Sprintf("%vprojects/%v/repos/%v/contents", defaultPathPrefix, projectName, repoName)

	if len(baseRevision) != 0 {
		u += fmt.Sprintf("?revision=%v", baseRevision)
	}

	body := push{CommitMessage: commitMessage, Changes: changes}

	req, err := con.client.newRequest(http.MethodPost, u, body)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	pushResult := new(PushResult)
	statusCode, err := con.client.do(ctx, req, pushResult)
	if err != nil {
		return nil, statusCode, err
	}
	return pushResult, statusCode, nil
}
