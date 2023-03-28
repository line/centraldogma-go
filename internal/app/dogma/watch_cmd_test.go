// Copyright 2021 LINE Corporation
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
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	dogma "go.linecorp.com/centraldogma"
)

func mockedCentralDogmaServer(entry dogma.Entry) *httptest.Server {
	revision := entry.Revision
	ts := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/revision/-1") {
				fmt.Fprintf(w, `{"revision": %d}`, revision)
				return
			}

			fmt.Fprintf(w, `{"revision": %[1]d,
			"entry": {"revision": %[1]d, "path": "%s", "content": %s, "type": "%s", "url": "%s"}}`,
				revision+1, entry.Path, string(entry.Content), entry.Type.String(), entry.URL)
		}))
	ts.StartTLS()
	return ts
}

func runCommandAndCaptureOutput(wc *watchCommand, f func(wc *watchCommand)) []byte {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	wc.out = w

	outChan := make(chan []byte)

	go func() {
		out, err := ioutil.ReadAll(r)
		if err != nil {
			panic(err)
		}
		outChan <- out
	}()

	f(wc)

	w.Close()

	return <-outChan
}

func TestListenerOption(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skipf("skipping %s due to a lack of cat command", t.Name())
	}
	if _, err := exec.LookPath("env"); err != nil {
		t.Skipf("skipping %s due to a lack of env command", t.Name())
	}

	entry := dogma.Entry{
		Content:  []byte(`{"foo":"FOO"}`),
		Path:     "/foo.json",
		Revision: 2,
		Type:     dogma.JSON,
		URL:      "/api/v1/projects/test/repos/test/contents/foo.json",
	}
	server := mockedCentralDogmaServer(entry)
	defer server.Close()

	client, _ := dogma.NewClientWithToken(server.URL, "anonymous", server.Client().Transport)

	wc := watchCommand{
		repo: repositoryRequestInfo{
			remoteURL: server.URL,
			projName:  "test",
			repoName:  "test",
			path:      "/foo.json",
			revision:  "-1",
		},
		listenerFile: "cat",
	}

	out := runCommandAndCaptureOutput(&wc, func(wc *watchCommand) { wc.executeWithDogmaClient(nil, client) })

	if !bytes.Equal(out, entry.Content) {
		t.Errorf("Got output %s; want %s", string(out), string(entry.Content))
	}

	wc.listenerFile = "env"

	out = runCommandAndCaptureOutput(&wc, func(wc *watchCommand) { wc.executeWithDogmaClient(nil, client) })
	env := string(out)

	if !strings.Contains(env, "DOGMA_WATCH_EVENT_PATH="+entry.Path) {
		t.Errorf("No path information found in environment %s", env)
	}
	if !strings.Contains(env, "DOGMA_WATCH_EVENT_REV="+strconv.FormatInt(int64(entry.Revision+1), 10)) {
		t.Errorf("No revision information found in environment %s", env)
	}
	if !strings.Contains(env, "DOGMA_WATCH_EVENT_CONTENT_TYPE="+entry.Type.String()) {
		t.Errorf("No content type information found in environment %s", env)
	}
	if !strings.Contains(env, "DOGMA_WATCH_EVENT_URL="+entry.URL) {
		t.Errorf("No url information found in environment %s", env)
	}
}

func TestInvalidListenerOption(t *testing.T) {
	entry := dogma.Entry{
		Content:  []byte(`{"foo":"FOO"}`),
		Path:     "/foo.json",
		Revision: 2,
		Type:     dogma.JSON,
		URL:      "/api/v1/projects/test/repos/test/contents/foo.json",
	}
	server := mockedCentralDogmaServer(entry)
	defer server.Close()

	client, _ := dogma.NewClientWithToken(server.URL, "anonymous", server.Client().Transport)

	wc := watchCommand{
		repo: repositoryRequestInfo{
			remoteURL: server.URL,
			projName:  "test",
			repoName:  "test",
			path:      "/foo.json",
			revision:  "-1",
		},
		listenerFile: "XYZ NO SUCH COMMAND",
	}

	err := wc.executeWithDogmaClient(nil, client)
	if _, ok := err.(*listenerExecError); !ok {
		t.Errorf("Didn't get listenerExecError; want = %v", err)
	}

}
