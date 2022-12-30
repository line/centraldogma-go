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
	"os"
	"reflect"
	"testing"
)

func TestNewLSCommand(t *testing.T) {
	defaultRemoteURL := "http://localhost:36462/"

	var tests = []struct {
		arguments []string
		revision  string
		want      interface{}
	}{
		{[]string{""}, "", lsProjectCommand{out: os.Stdout, remoteURL: defaultRemoteURL}},
		{[]string{"/foo/"}, "", lsRepositoryCommand{out: os.Stdout, remoteURL: defaultRemoteURL, projName: "foo"}},
		{[]string{"foo/"}, "", lsRepositoryCommand{out: os.Stdout, remoteURL: defaultRemoteURL, projName: "foo"}},
		{[]string{"foo"}, "", lsRepositoryCommand{out: os.Stdout, remoteURL: defaultRemoteURL, projName: "foo"}},
		{[]string{"foo/bar"}, "",
			lsPathCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL,
					projName:  "foo",
					repoName:  "bar",
					path:      "/",
					revision:  "-1",
				},
			},
		},
		{[]string{"foo/bar/"}, "",
			lsPathCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL,
					projName:  "foo",
					repoName:  "bar",
					path:      "/",
					revision:  "-1",
				},
			},
		},
		{[]string{"foo/bar/a"}, "",
			lsPathCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL,
					projName:  "foo",
					repoName:  "bar",
					path:      "/a",
					revision:  "-1",
				},
			},
		},
		{[]string{"foo/bar/a/"}, "100",
			lsPathCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL,
					projName:  "foo",
					repoName:  "bar",
					path:      "/a/",
					revision:  "100",
				},
			},
		},
		{[]string{"foo/bar/a.txt"}, "",
			lsPathCommand{
				out: os.Stdout,
				repo: repositoryRequestInfo{
					remoteURL: defaultRemoteURL,
					projName:  "foo",
					repoName:  "bar",
					path:      "/a.txt",
					revision:  "-1",
				},
			},
		},
	}

	for _, test := range tests {
		c := newContext(test.arguments, defaultRemoteURL, test.revision)

		got, _ := newLSCommand(c, os.Stdout, 0)
		switch comType := got.(type) {
		case *lsProjectCommand:
			got2 := lsProjectCommand(*comType)
			if !reflect.DeepEqual(got2, test.want) {
				t.Errorf("newLSCommand(%q) = %q, want: %q", test.arguments, got2, test.want)
			}
		case *lsRepositoryCommand:
			got2 := lsRepositoryCommand(*comType)
			if !reflect.DeepEqual(got2, test.want) {
				t.Errorf("newLSCommand(%q) = %q, want: %q", test.arguments, got2, test.want)
			}
		case *lsPathCommand:
			got2 := lsPathCommand(*comType)
			if !reflect.DeepEqual(got2, test.want) {
				t.Errorf("newAddCommand(%+v) = %+v, want: %+v", test.arguments, got2, test.want)
			}
		default:
			t.Errorf("newLSCommand(%q) = %q, want: %q", test.arguments, got, test.want)
		}
	}
}
