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
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/urfave/cli/v2"
	"go.linecorp.com/centraldogma"
)

type rmFileCommand struct {
	out  io.Writer
	repo repositoryRequestInfo
}

func (rf *rmFileCommand) execute(c *cli.Context) error {
	repo := rf.repo
	client, err := newDogmaClient(c, repo.remoteURL)
	if err != nil {
		return err
	}

	change := &centraldogma.Change{Path: repo.path, Type: centraldogma.Remove}

	commitMessage, err := getCommitMessage(c, rf.out, repo.path, removal)
	if err != nil {
		return err
	}

	_, httpStatusCode, err := client.Push(context.Background(),
		repo.projName, repo.repoName, repo.revision, commitMessage, []*centraldogma.Change{change})
	if err != nil {
		return err
	}
	if httpStatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete the file: /%s/%s%s revision: %q (status: %d)",
			repo.projName, repo.repoName, repo.path, repo.revision, httpStatusCode)
	}

	fmt.Fprintf(rf.out, "Deleted: /%s/%s%s\n", repo.projName, repo.repoName, repo.path)
	return nil
}

func newRMCommand(c *cli.Context, out io.Writer) (Command, error) {
	repo, err := newRepositoryRequestInfo(c)
	if err != nil {
		return nil, err
	}

	return &rmFileCommand{out: out, repo: repo}, nil
}
