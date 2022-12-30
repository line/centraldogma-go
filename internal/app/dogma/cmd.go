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

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/urfave/cli"
	"go.linecorp.com/centraldogma"
)

// Command is the common interface implemented by all commands.
type Command interface {
	// execute executes the command
	execute(c *cli.Context) error
}

func getRemoteURL(remoteURL string) (string, error) {
	if len(remoteURL) == 0 {
		fmt.Println("Enter server address: (e.g. http://example.com:36462)")
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return "", errors.New("you must input specific address (e.g. http://example.com:36462)")
		}
		line := strings.TrimSpace(scanner.Text())
		if _, err := url.Parse(line); err != nil {
			return "", errors.New("invalid server address")
		}
		return line, nil
	}
	return remoteURL, nil
}

type repositoryRequestInfo struct {
	remoteURL           string
	projName            string
	repoName            string
	path                string
	revision            string
	isRecursiveDownload bool
}

// newRepositoryRequestInfo creates a repositoryRequestInfo.
func newRepositoryRequestInfo(c *cli.Context) (repositoryRequestInfo, error) {
	repo := repositoryRequestInfo{path: "/", revision: "-1"}
	if len(c.Args()) == 0 {
		return repo, newCommandLineError(c)
	}
	split := splitPath(c.Args().First())
	if len(split) < 2 { // Need at least projName and repoName.
		return repo, newCommandLineError(c)
	}

	remoteURL, err := getRemoteURL(c.Parent().String("connect"))
	if err != nil {
		return repo, err
	}
	repo.remoteURL = remoteURL
	repo.projName = split[0]
	repo.repoName = split[1]

	if len(split) > 2 && len(split[2]) != 0 {
		repo.path = split[2]
	}

	revision := c.String("revision")
	if len(revision) != 0 {
		repo.revision = revision
	}

	repo.isRecursiveDownload = c.Bool("recursive")
	return repo, nil
}

// splitPath parses the path into projName, repoName and path.
func splitPath(fullPath string) []string {
	endsWithSlash := false
	if strings.HasSuffix(fullPath, "/") {
		endsWithSlash = true
	}

	split := strings.Split(fullPath, "/")
	var ret []string
	for _, str := range split {
		if str != "" {
			ret = append(ret, str)
		}
	}
	if len(ret) <= 2 {
		return ret
	}
	repositoryPath := "/" + strings.Join(ret[2:], "/")
	if endsWithSlash {
		repositoryPath += "/"
	}
	return append(ret[:2], repositoryPath)
}

type repositoryRequestInfoWithFromTo struct {
	remoteURL string
	projName  string
	repoName  string
	path      string
	from      string
	to        string
}

// getRemoteFileEntry downloads the entry of the specified remote path. If the jsonPaths
// is specified, only the applied content of the jsonPaths will be downloaded.
func getRemoteFileEntry(c *cli.Context,
	remoteURL, projName, repoName, repoPath, revision string, jsonPaths []string) (*centraldogma.Entry, error) {
	client, err := newDogmaClient(c, remoteURL)
	if err != nil {
		return nil, err
	}

	return getRemoteFileEntryWithDogmaClient(client,
		projName, repoName, repoPath, revision, jsonPaths)
}

func getRemoteFileEntryWithDogmaClient(client *centraldogma.Client,
	projName, repoName, repoPath, revision string, jsonPaths []string) (*centraldogma.Entry, error) {
	query := createQuery(repoPath, jsonPaths)
	entry, httpStatusCode, err := client.GetFile(context.Background(), projName, repoName, revision, query)
	if err != nil {
		return nil, err
	}

	if httpStatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get the file: /%s/%s%s revision: %q (status: %d)",
			projName, repoName, repoPath, revision, httpStatusCode)
	}

	return entry, nil
}

func newDogmaClient(c *cli.Context, baseURL string) (client *centraldogma.Client, err error) {
	enabled, err := checkIfSecurityEnabled(baseURL)
	if err != nil {
		return nil, err
	}

	if !enabled {
		// Create a client with the anonymous token.
		return centraldogma.NewClientWithToken(baseURL, "anonymous", nil)
	}

	token := c.Parent().String("token")
	if len(token) != 0 {
		if client, err = centraldogma.NewClientWithToken(baseURL, token, nil); err != nil {
			return nil, err
		}
	} else {
		return nil, cli.NewExitError("You must specify a token using '--token'.", 1)
	}

	return client, nil
}

func createQuery(repoPath string, jsonPaths []string) *centraldogma.Query {
	if len(jsonPaths) != 0 && strings.HasSuffix(strings.ToLower(repoPath), "json") {
		return &centraldogma.Query{Path: repoPath, Type: centraldogma.JSONPath, Expressions: jsonPaths}
	} else {
		return &centraldogma.Query{Path: repoPath, Type: centraldogma.Identity}
	}
}

func checkIfSecurityEnabled(baseURL string) (bool, error) {
	// Create a client with the anonymous token just to check the security is enabled.
	client, err := centraldogma.NewClientWithToken(baseURL, "anonymous", nil)
	if err != nil {
		return false, err
	}
	return client.SecurityEnabled()
}
