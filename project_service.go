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
)

type projectService service

// Project represents a project in the Central Dogma server.
type Project struct {
	Name      string `json:"name"`
	Creator   Author `json:"creator,omitempty"`
	URL       string `json:"url,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

func (p *projectService) create(ctx context.Context, name string) (*Project, int, error) {
	u := defaultPathPrefix + "projects"

	body := map[string]string{"name": name}
	req, err := p.client.newRequest(http.MethodPost, u, body)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	project := new(Project)
	httpStatusCode, err := p.client.do(ctx, req, project, false)
	if err != nil {
		return nil, httpStatusCode, err
	}
	return project, httpStatusCode, nil
}

func (p *projectService) remove(ctx context.Context, name string) (int, error) {
	u := defaultPathPrefix + "projects/" + name

	req, err := p.client.newRequest(http.MethodDelete, u, nil)
	if err != nil {
		return UnknownHttpStatusCode, err
	}

	httpStatusCode, err := p.client.do(ctx, req, nil, false)
	if err != nil {
		return httpStatusCode, err
	}
	return httpStatusCode, nil
}

func (p *projectService) purge(ctx context.Context, name string) (int, error) {
	u := defaultPathPrefix + "projects/" + name + "/removed"

	req, err := p.client.newRequest(http.MethodDelete, u, nil)
	if err != nil {
		return UnknownHttpStatusCode, err
	}

	httpStatusCode, err := p.client.do(ctx, req, nil, false)
	if err != nil {
		return httpStatusCode, err
	}
	return httpStatusCode, nil
}

func (p *projectService) unremove(ctx context.Context, name string) (*Project, int, error) {
	u := defaultPathPrefix + "projects/" + name

	req, err := p.client.newRequest(http.MethodPatch, u, `[{"op":"replace", "path":"/status", "value":"active"}]`)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	project := new(Project)
	httpStatusCode, err := p.client.do(ctx, req, project, false)
	if err != nil {
		return nil, httpStatusCode, err
	}
	return project, httpStatusCode, nil
}

func (p *projectService) list(ctx context.Context) ([]*Project, int, error) {
	u := defaultPathPrefix + "projects"

	req, err := p.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	var projects []*Project
	httpStatusCode, err := p.client.do(ctx, req, &projects, false)
	if err != nil {
		return nil, httpStatusCode, err
	}
	return projects, httpStatusCode, nil
}

func (p *projectService) listRemoved(ctx context.Context) ([]*Project, int, error) {
	u := defaultPathPrefix + "projects?status=removed"

	req, err := p.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, UnknownHttpStatusCode, err
	}

	var projects []*Project
	httpStatusCode, err := p.client.do(ctx, req, &projects, false)
	if err != nil {
		return nil, httpStatusCode, err
	}
	return projects, httpStatusCode, nil
}
