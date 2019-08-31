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
	"net/http"
	"net/url"
	"path"
)

type repositoryService service

// Repository represents a repository in the Central Dogma server.
type Repository struct {
	Name         string `json:"name"`
	Creator      Author `json:"creator,omitempty"`
	HeadRevision int    `json:"headRevision,omitempty"`
	URL          string `json:"url,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
}

func (r *repositoryService) create(ctx context.Context, projectName, repoName string) (*Repository, int, error) {
	// build relative url
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		projects, projectName,
		repos,
	))
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	body := map[string]string{"name": repoName}
	req, err := r.client.newRequest(http.MethodPost, u, body)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	repo := new(Repository)
	httpStatusCode, err := r.client.do(ctx, req, repo, false)
	if err != nil {
		return nil, httpStatusCode, err
	}

	return repo, httpStatusCode, nil
}

func (r *repositoryService) remove(ctx context.Context, projectName, repoName string) (int, error) {
	// build relative url
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		projects, projectName,
		repos, repoName,
	))
	if err != nil {
		return UnknownHttpStatusCode, err
	}

	req, err := r.client.newRequest(http.MethodDelete, u, nil)
	if err != nil {
		return UnknownHttpStatusCode, err
	}

	httpStatusCode, err := r.client.do(ctx, req, nil, false)
	if err != nil {
		return httpStatusCode, err
	}
	return httpStatusCode, nil
}

func (r *repositoryService) purge(ctx context.Context, projectName, repoName string) (int, error) {
	// build relative url
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		projects, projectName,
		repos, repoName,
		actionRemoved,
	))
	if err != nil {
		return UnknownHttpStatusCode, err
	}

	req, err := r.client.newRequest(http.MethodDelete, u, nil)
	if err != nil {
		return UnknownHttpStatusCode, err
	}

	httpStatusCode, err := r.client.do(ctx, req, nil, false)
	if err != nil {
		return httpStatusCode, err
	}
	return httpStatusCode, nil
}

func (r *repositoryService) unremove(ctx context.Context, projectName, repoName string) (*Repository, int, error) {
	// build relative url
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		projects, projectName,
		repos, repoName,
	))
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	req, err := r.client.newRequest(http.MethodPatch, u, `[{"op":"replace", "path":"/status", "value":"active"}]`)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	repo := new(Repository)
	httpStatusCode, err := r.client.do(ctx, req, repo, false)
	if err != nil {
		return nil, httpStatusCode, err
	}
	return repo, httpStatusCode, nil
}

func (r *repositoryService) list(ctx context.Context, projectName string) ([]*Repository, int, error) {
	// build relative url
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		projects, projectName,
		repos,
	))
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	req, err := r.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	var repos []*Repository
	httpStatusCode, err := r.client.do(ctx, req, &repos, false)
	if err != nil {
		return nil, httpStatusCode, err
	}
	return repos, httpStatusCode, nil
}

func (r *repositoryService) listRemoved(ctx context.Context, projectName string) ([]*Repository, int, error) {
	// build relative url
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		projects, projectName,
		repos,
	))
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	// build query params
	q := u.Query()
	q.Set("status", "removed")
	u.RawQuery = q.Encode()

	req, err := r.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	var repos []*Repository
	httpStatusCode, err := r.client.do(ctx, req, &repos, false)
	if err != nil {
		return nil, httpStatusCode, err
	}
	return repos, httpStatusCode, nil
}

func (r *repositoryService) normalizeRevision(
	ctx context.Context, projectName, repoName, revision string) (int, int, error) {
	// build relative url
	u, err := url.Parse(path.Join(
		defaultPathPrefix,
		projects, projectName,
		repos, repoName,
		"revision", revision,
	))
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	req, err := r.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return -1, UnknownHttpStatusCode, err
	}

	rev := new(rev)
	httpStatusCode, err := r.client.do(ctx, req, rev, false)
	if err != nil {
		return -1, httpStatusCode, err
	}
	return rev.Rev, httpStatusCode, nil
}

type rev struct {
	Rev int `json:"revision"`
}
