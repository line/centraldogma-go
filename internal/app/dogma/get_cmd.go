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
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/urfave/cli"
	"go.linecorp.com/centraldogma"
)

const (
	defaultPermMode = 0755
)

// A getFileCommand fetches the content of the file in the specified path matched by the
// JSON path expressions with the specified revision.
type getFileCommand struct {
	out           io.Writer
	repo          repositoryRequestInfo
	localFilePath string
	jsonPaths     []string
}

func (gf *getFileCommand) execute(c *cli.Context) error {
	repo := gf.repo

	entry, err := getRemoteEntry(c, &repo, repo.path, gf.jsonPaths)
	if err != nil {
		return err
	}

	if entry.Type == centraldogma.Directory && !repo.isRecursiveDownload {
		return fmt.Errorf("%+q is a directory, you might want to use `--recursive` instead", repo.path)
	}

	filePath := creatableFilePath(gf.localFilePath, 1)
	fd, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer fd.Close()

	if entry.Type == centraldogma.JSON {
		b := safeMarshalIndent(entry.Content)
		if _, err = fd.Write(b); err != nil {
			return err
		}
	} else if entry.Type == centraldogma.Text {
		_, err = fd.Write(entry.Content)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(gf.out, "Downloaded: %s\n", path.Base(filePath))
	return nil
}

type getDirectoryCommand struct {
	out           io.Writer
	repo          repositoryRequestInfo
	localFilePath string
	jsonPaths     []string
}

func (gd *getDirectoryCommand) execute(c *cli.Context) error {
	client, err := newDogmaClient(c, gd.repo.remoteURL)
	if err != nil {
		return err
	}

	return gd.executeWithDogmaClient(c, client)
}

func (gd *getDirectoryCommand) executeWithDogmaClient(_ *cli.Context, client *centraldogma.Client) error {
	repo := gd.repo
	entry, err := getRemoteFileEntryWithDogmaClient(client,
		repo.projName, repo.repoName, repo.path, repo.revision, gd.jsonPaths)
	if err != nil {
		return err
	}

	if entry.Type != centraldogma.Directory && repo.isRecursiveDownload {
		return fmt.Errorf("%+q is not a directory, you might want to remove `--recursive` instead", repo.path)
	}

	basename := creatableFilePath(gd.localFilePath, 1)
	if err := os.MkdirAll(basename, defaultPermMode); err != nil {
		return err
	}
	return gd.recurseDownload(context.Background(), client, basename, entry)
}

func (gd *getDirectoryCommand) recurseDownload(ctx context.Context, client *centraldogma.Client,
	basename string, rootEntry *centraldogma.Entry) error {
	if rootEntry.Type != centraldogma.Directory {
		return fmt.Errorf("%+q is not a directory, you might want to remove `--recursive` instead",
			rootEntry.Path)
	}

	repo := gd.repo
	path := rootEntry.Path
	entries, httpStatusCode, err := client.ListFiles(ctx, repo.projName, repo.repoName, repo.revision, path)
	if err != nil {
		return err
	}

	if httpStatusCode != http.StatusOK {
		return fmt.Errorf("failed to get the list of files in the /%s/%s%s revision: %q (status: %d)",
			repo.projName, repo.repoName, path, repo.revision, httpStatusCode)
	}

	for _, entry := range entries {
		switch entry.Type {
		case centraldogma.Directory:
			if err := gd.recurseDownload(ctx, client, basename, entry); err != nil {
				return err
			}
		default:
			if err := gd.downloadFile(client, basename, entry.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (gd *getDirectoryCommand) downloadFile(client *centraldogma.Client, basename, path string) error {
	repo := gd.repo
	name := gd.constructFilename(basename, path)
	if err := os.MkdirAll(filepath.Dir(name), defaultPermMode); err != nil {
		return err
	}
	fd, err := os.Create(name)
	if err != nil {
		return err
	}
	defer fd.Close()

	entry, err := getRemoteFileEntryWithDogmaClient(client,
		repo.projName, repo.repoName, path, repo.revision, gd.jsonPaths)
	if err != nil {
		return err
	}

	if entry.Type == centraldogma.JSON {
		b := safeMarshalIndent(entry.Content)
		if _, err = fd.Write(b); err != nil {
			return err
		}
	} else if entry.Type == centraldogma.Text {
		if _, err = fd.Write(entry.Content); err != nil {
			return err
		}
	}

	fmt.Fprintf(gd.out, "Downloaded: %s\n", name)

	return nil
}

func (gd *getDirectoryCommand) constructFilename(basename, path string) string {
	paths := strings.Split(path, "/")
	cleanPath := strings.Join(paths[2:], "/")
	return basename + "/" + cleanPath
}

func getRemoteEntry(c *cli.Context, repo *repositoryRequestInfo, path string, jsonPaths []string) (*centraldogma.Entry, error) {
	entry, err := getRemoteFileEntry(
		c, repo.remoteURL, repo.projName, repo.repoName, path, repo.revision, jsonPaths)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// A catFileCommand shows the content of the file in the specified path matched by the
// JSON path expressions with the specified revision.
type catFileCommand struct {
	out       io.Writer
	repo      repositoryRequestInfo
	jsonPaths []string
}

func (cf *catFileCommand) execute(c *cli.Context) error {
	repo := cf.repo
	entry, err := getRemoteFileEntry(
		c, repo.remoteURL, repo.projName, repo.repoName, repo.path, repo.revision, cf.jsonPaths)
	if err != nil {
		return err
	}

	if entry.Type == centraldogma.JSON {
		b := safeMarshalIndent(entry.Content)
		fmt.Fprintf(cf.out, "%s\n", string(b))
	} else if entry.Type == centraldogma.Text { //
		fmt.Fprintf(cf.out, "%s\n", string(entry.Content))
	}

	return nil
}

func creatableFilePath(filePath string, inc int) string {
	regex, _ := regexp.Compile(`\.[0-9]*$`)
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		if inc == 1 {
			filePath += "."
		}
		startIndex := regex.FindStringIndex(filePath)
		filePath = filePath[0:startIndex[0]+1] + strconv.Itoa(inc)
		inc++
		return creatableFilePath(filePath, inc)
	}
	return filePath
}

// newGetCommand creates the getCommand. If the localFilePath is not specified, the file name of the path
// will be set by default.
func newGetCommand(c *cli.Context, out io.Writer) (Command, error) {
	repo, err := newRepositoryRequestInfo(c)
	if err != nil {
		return nil, err
	}

	localFilePath := path.Base(repo.path)
	if len(c.Args()) == 2 && len(c.Args().Get(1)) != 0 {
		localFilePath = c.Args().Get(1)
	}

	if repo.isRecursiveDownload {
		return &getDirectoryCommand{
			out:           out,
			repo:          repo,
			localFilePath: localFilePath,
			jsonPaths:     c.StringSlice("jsonpath"),
		}, nil
	}

	return &getFileCommand{out: out, repo: repo, localFilePath: localFilePath, jsonPaths: c.StringSlice("jsonpath")}, nil
}

// newCatCommand creates the catCommand.
func newCatCommand(c *cli.Context, out io.Writer) (Command, error) {
	repo, err := newRepositoryRequestInfo(c)
	if err != nil {
		return nil, err
	}
	return &catFileCommand{out: out, repo: repo, jsonPaths: c.StringSlice("jsonpath")}, nil
}
