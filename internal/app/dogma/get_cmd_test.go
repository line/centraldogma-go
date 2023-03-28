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
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"go.linecorp.com/centraldogma"
)

func TestNewGetCommand(t *testing.T) {
	defaultRemoteURL := "http://localhost:36462/"

	var tests = []struct {
		arguments   []string
		revision    string
		isRecursive bool
		want        interface{}
	}{
		{[]string{"foo/bar/a.txt"}, "", false,
			getFileCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL, projName: "foo", repoName: "bar",
					path: "/a.txt", revision: "-1"},
				localFilePath: "a.txt"}},

		{[]string{"foo/bar/b/a.txt"}, "10", false,
			getFileCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL, projName: "foo", repoName: "bar",
					path: "/b/a.txt", revision: "10"},
				localFilePath: "a.txt"}},

		{[]string{"foo/bar/b/a.txt", "c.txt"}, "", false,
			getFileCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL, projName: "foo", repoName: "bar",
					path: "/b/a.txt", revision: "-1"},
				localFilePath: "c.txt"}},

		{[]string{"foo/bar/a.txt", "b/c.txt"}, "", false,
			getFileCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL, projName: "foo", repoName: "bar",
					path: "/a.txt", revision: "-1"},
				localFilePath: "b/c.txt"}},
		{
			arguments:   []string{"foo/bar/a.txt"},
			revision:    "",
			isRecursive: true,
			want: getDirectoryCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL, projName: "foo", repoName: "bar",
					path: "/a.txt", revision: "-1",
					isRecursiveDownload: true,
				},
				localFilePath: "a.txt",
			},
		},
		{
			arguments:   []string{"foo/bar/**"},
			revision:    "",
			isRecursive: true,
			want: getDirectoryCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL, projName: "foo", repoName: "bar",
					path: "/**", revision: "-1",
					isRecursiveDownload: true,
				},
				localFilePath: "bar",
			},
		},
		{
			arguments:   []string{"foo/bar/f**"},
			revision:    "",
			isRecursive: true,
			want: getDirectoryCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL, projName: "foo", repoName: "bar",
					path: "/f**", revision: "-1",
					isRecursiveDownload: true,
				},
				localFilePath: "f**",
			},
		},
		{
			arguments:   []string{"foo/bar/f**/**"},
			revision:    "",
			isRecursive: true,
			want: getDirectoryCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL, projName: "foo", repoName: "bar",
					path: "/f**/**", revision: "-1",
					isRecursiveDownload: true,
				},
				localFilePath: "f**",
			},
		},
		{
			arguments:   []string{"foo/bar/**/baz"},
			revision:    "",
			isRecursive: true,
			want: getDirectoryCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL, projName: "foo", repoName: "bar",
					path: "/**/baz", revision: "-1",
					isRecursiveDownload: true,
				},
				localFilePath: "baz",
			},
		},
	}

	for _, test := range tests {
		c := newGetCmdContext(test.arguments, defaultRemoteURL, test.revision, test.isRecursive)

		got, _ := newGetCommand(c, os.Stdout)
		switch comType := got.(type) {
		case *getFileCommand:
			got2 := getFileCommand(*comType)
			if !reflect.DeepEqual(got2, test.want) {
				t.Errorf("newGetCommand(%+v) = %+v, want: %+v", test.arguments, got2, test.want)
			}
		case *getDirectoryCommand:
			got2 := getDirectoryCommand(*comType)
			if !reflect.DeepEqual(got2, test.want) {
				t.Errorf("newGetCommand(%+v) = %+v, want: %+v", test.arguments, got2, test.want)
			}
		default:
			t.Errorf("newGetCommand(%q) = %q, want: %q", test.arguments, got, test.want)
		}
	}
}

var mockServer *httptest.Server

func mockedCentralDogmaServerForRecursive() *httptest.Server {
	responseMap := map[string]string{
		// file
		"/contents/x":     `{"revision":1,"path":"/x","type":"DIRECTORY","url":"/api/v1/projects/abcd/repos/repo1/contents/x"}`,
		"/contents/x/y":   `{"revision":1,"path":"/x/y","type":"DIRECTORY","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y"}`,
		"/contents/x/y/z": `{"revision":1,"path":"/x/y/z","type":"DIRECTORY","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/z"}`,

		"/contents/x/foo.json":     `{"revision":1,"path":"/x/foo.json","type":"JSON","content":{"name":"abcd/repo1/x/foo.json"},"url":"/api/v1/projects/abcd/repos/repo1/contents/x/foo.json"}`,
		"/contents/x/y/foo.json":   `{"revision":1,"path":"/x/y/foo.json","type":"JSON","content":{"name":"abcd/repo1/x/y/foo.json"},"url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/foo.json"}`,
		"/contents/x/y/z/foo.json": `{"revision":1,"path":"/x/y/z/foo.json","type":"JSON","content":{"name":"abcd/repo1/x/y/z/foo.json"},"url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/z/foo.json"}`,

		// dir
		"/list/x": `[
			{"revision":1,"path":"/x/foo.json","type":"JSON","url":"/api/v1/projects/abcd/repos/repo1/contents/x/foo.json"},
			{"revision":1,"path":"/x/y","type":"DIRECTORY","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y"}
		]`,

		"/list/x/y": `[
			{"revision":1,"path":"/x/y/foo.json","type":"JSON","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/foo.json"},
			{"revision":1,"path":"/x/y/z","type":"DIRECTORY","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/z"}
		]`,

		"/list/x/y/z": `[{"revision":1,"path":"/x/y/z/foo.json","type":"JSON","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/z/foo.json"}]`,

		// glob
		"/list/x/**": `[
			{"revision":1,"path":"/x/foo.json","type":"JSON","url":"/api/v1/projects/abcd/repos/repo1/contents/x/foo.json"},
			{"revision":1,"path":"/x/y","type":"DIRECTORY","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y"},
			{"revision":1,"path":"/x/y/foo.json","type":"JSON","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/foo.json"},
			{"revision":1,"path":"/x/y/z","type":"DIRECTORY","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/z"},
			{"revision":1,"path":"/x/y/z/foo.json","type":"JSON","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/z/foo.json"}
		]`,
		"/list/x/y/**": `[
			{"revision":1,"path":"/x/y/foo.json","type":"JSON","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/foo.json"},
			{"revision":1,"path":"/x/y/z","type":"DIRECTORY","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/z"},
			{"revision":1,"path":"/x/y/z/foo.json","type":"JSON","url":"/api/v1/projects/abcd/repos/repo1/contents/x/y/z/foo.json"}
		]`,
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/abcd/repos/repo1")
		fmt.Fprint(w, responseMap[path])
	}))
}

func TestMain(m *testing.M) {
	mockServer = mockedCentralDogmaServerForRecursive()

	exitCode := m.Run()
	mockServer.Close()

	os.Exit(exitCode)
}

func TestGetRecursive(t *testing.T) {
	client, err := centraldogma.NewClientWithToken(mockServer.URL, "anonymous", mockServer.Client().Transport)
	if err != nil {
		t.Errorf(err.Error())
	}

	tt := []struct {
		_        struct{}
		Name     string
		ProjName string
		RepoName string
		Path     string
		Targets  []string
	}{
		{
			Name:     "download x",
			ProjName: "abcd",
			RepoName: "repo1",
			Path:     "/x",
			Targets: []string{
				"/foo.json",
				"/y/foo.json",
				"/y/z/foo.json",
			},
		},
		{
			Name:     "download x/**",
			ProjName: "abcd",
			RepoName: "repo1",
			Path:     "/x/**",
			Targets: []string{
				"/foo.json",
				"/y/foo.json",
				"/y/z/foo.json",
			},
		},
		{
			Name:     "download x/y/**",
			ProjName: "abcd",
			RepoName: "repo1",
			Path:     "/x/y/**",
			Targets: []string{
				"/foo.json",
				"/z/foo.json",
			},
		},
	}

	for i := range tt {
		tc := tt[i]

		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			b := make([]byte, 5)
			rand.Read(b)
			localFilePath := "/tmp/" + hex.EncodeToString(b)
			defer os.RemoveAll(localFilePath)

			flagArgs := []string{
				fmt.Sprintf("%s/%s%s", tc.ProjName, tc.RepoName, tc.Path),
				localFilePath,
			}
			c := newGetCmdContext(flagArgs, mockServer.URL, "", true)
			cmd := &getDirectoryCommand{
				out: bufio.NewWriter(new(bytes.Buffer)),
				repo: repositoryRequestInfo{
					remoteURL:           mockServer.URL,
					projName:            tc.ProjName,
					repoName:            tc.RepoName,
					path:                tc.Path,
					revision:            "",
					isRecursiveDownload: true,
				},
				localFilePath: localFilePath,
			}

			c.Context = putDogmaClientTo(c.Context, client)
			if err := cmd.execute(c); err != nil {
				t.Errorf(err.Error())
			}

			for _, target := range tc.Targets {
				downloadedFile := localFilePath + target
				if _, err := os.Stat(downloadedFile); err != nil {
					t.Errorf("downloaded: %+q file is expected to be exists: %s", downloadedFile, err.Error())
				}

				b, err := os.ReadFile(downloadedFile)
				if err != nil {
					t.Error(err.Error())
				}

				m := make(map[string]string)
				if err := json.Unmarshal(b, &m); err != nil {
					t.Error(err.Error())
				}
				if !strings.HasSuffix(m["name"], target) {
					t.Errorf("%+q content's name is expected to ended with: %+q, got: %+q",
						downloadedFile, target, m["name"])
				}
			}
		})
	}
}

func TestGetDirectoryCommand_constructFilename(t *testing.T) {
	cmd := &getDirectoryCommand{}

	const basename = "base"

	tt := []struct {
		_                struct{}
		Name             string
		Path             string
		UserQueryPath    string
		ExpectedFilename string
	}{
		{
			Name:             "download foo from foo",
			Path:             "/foo/foo.json",
			UserQueryPath:    "/foo",
			ExpectedFilename: "base/foo.json",
		},
		{
			Name:             "download foo-bar from foo",
			Path:             "/foo/bar/bar.json",
			UserQueryPath:    "/foo",
			ExpectedFilename: "base/bar/bar.json",
		},
		{
			Name:             "download foo-bar-baz from foo",
			Path:             "/foo/bar/baz/baz.json",
			UserQueryPath:    "/foo",
			ExpectedFilename: "base/bar/baz/baz.json",
		},
		{
			Name:             "download foo-bar from bar",
			Path:             "/foo/bar/bar.json",
			UserQueryPath:    "/foo/bar",
			ExpectedFilename: "base/bar.json",
		},
		{
			Name:             "download foo-bar-baz from bar",
			Path:             "/foo/bar/baz/baz.json",
			UserQueryPath:    "/foo/bar",
			ExpectedFilename: "base/baz/baz.json",
		},
		{
			Name:             "download foo-bar-baz from baz",
			Path:             "/foo/bar/baz/baz.json",
			UserQueryPath:    "/foo/bar/baz",
			ExpectedFilename: "base/baz.json",
		},
	}

	for i := range tt {
		tc := tt[i]
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			actualFilename, actualError := cmd.constructFilename(basename, tc.Path, tc.UserQueryPath)
			if actualError != nil {
				t.Errorf("not expecting any error: %+q", actualError)
				return
			}

			if actualFilename != tc.ExpectedFilename {
				t.Errorf("expected: %+q, actual: %+q", tc.ExpectedFilename, actualFilename)
				return
			}
		})
	}
}
