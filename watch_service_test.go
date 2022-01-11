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
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"
)

var response = `{"revision":3,
"entry":{"path":"/a.json", "type":"JSON", "content": {"a":"b"}}
}`

func TestWatchFile(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	notModifiedResponse := true

	mux.HandleFunc("/api/v1/projects/foo/repos/bar/contents/a.json",
		func(w http.ResponseWriter, r *http.Request) {
			testMethod(t, r, http.MethodGet)
			testHeader(t, r, "if-none-match", "1")
			testHeader(t, r, "prefer", "wait=1")

			// Let's pretend that the content is modified after 100 Millisecond.
			time.Sleep(100 * time.Millisecond)
			if notModifiedResponse {
				w.WriteHeader(http.StatusNotModified)
				notModifiedResponse = false
			} else {
				fmt.Fprint(w, response)
			}
		})

	query := &Query{Path: "/a.json", Type: Identity}
	watchResult, closer, _ := c.WatchFile(context.Background(), "foo", "bar", query, 1*time.Second)
	defer closer()

	entryWant := Entry{Path: "/a.json", Type: JSON, Content: EntryContent(`{"a":"b"}`)}
	revisionWant := 3
	select {
	case result := <-watchResult:
		if !reflect.DeepEqual(result.Revision, revisionWant) {
			t.Errorf("WatchFile returned %+v, want %+v", result.Revision, revisionWant)
		}
		if !reflect.DeepEqual(result.Entry, entryWant) {
			t.Errorf("WatchFile returned %+v, want %+v", result.Entry, entryWant)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("WatchFile returned nothing, want %+v, %+v", revisionWant, entryWant)
	}
}

func TestWatchFileInvalidPath(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	mux.HandleFunc("/api/v1/projects/foo/repos/bar/contents/a.json",
		func(w http.ResponseWriter, r *http.Request) {
			testMethod(t, r, http.MethodGet)
			testHeader(t, r, "if-none-match", "1")
			testHeader(t, r, "prefer", "wait=1")

			// Let's pretend that the content is modified after 100 Millisecond.
			time.Sleep(100 * time.Millisecond)
			fmt.Fprint(w, response)
		})

	query := &Query{Path: "a.json", Type: Identity}
	watchResult, closer, _ := c.WatchFile(context.Background(), "foo", "bar", query, 1*time.Second)
	defer closer()

	entryWant := Entry{Path: "/a.json", Type: JSON, Content: EntryContent(`{"a":"b"}`)}
	revisionWant := 3
	select {
	case result := <-watchResult:
		if !reflect.DeepEqual(result.Revision, revisionWant) {
			t.Errorf("WatchFile returned %+v, want %+v", result.Revision, revisionWant)
		}
		if !reflect.DeepEqual(result.Entry, entryWant) {
			t.Errorf("WatchFile returned %+v, want %+v", result.Entry, entryWant)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("WatchFile returned nothing, want %+v, %+v", revisionWant, entryWant)
	}
}

func TestWatcher(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	expectedLastKnownRevision := 1
	handler := func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, http.MethodGet)
		testHeader(t, r, "if-none-match", strconv.Itoa(expectedLastKnownRevision))
		testHeader(t, r, "prefer", "wait=60") // watchTimeout is 60 seconds

		// Let's pretend that the content is modified after 100 millisecond and the revision is increased by 1.
		time.Sleep(100 * time.Millisecond)
		expectedLastKnownRevision++

		fmt.Fprint(w, `{"revision":`+strconv.Itoa(expectedLastKnownRevision)+`,
"entry":{"path":"/a.json", "type":"JSON", "content": {"a":`+strconv.Itoa(expectedLastKnownRevision)+`}}
}`)
	}

	mux.HandleFunc("/api/v1/projects/foo/repos/bar/contents/a.json", handler)

	query := &Query{Path: "/a.json", Type: Identity}
	fw, _ := c.FileWatcher("foo", "bar", query)
	defer fw.Close()

	myCh1 := make(chan WatchResult, 128)
	myCh2 := make(chan WatchResult, 128)
	listener1 := func(value WatchResult) { myCh1 <- value }
	listener2 := func(value WatchResult) { myCh2 <- value }

	_ = fw.Watch(listener1)
	_ = fw.Watch(listener2)

	want := 2
	for i := 0; i < 5; i++ {
		testChannelValue(t, myCh1, want)
		testChannelValue(t, myCh2, want)
		want++
	}
}

func testChannelValue(t *testing.T, myCh <-chan WatchResult, want int) {
	select {
	case value := <-myCh:
		aStruct := struct {
			A int `json:"a"`
		}{}
		_ = json.Unmarshal(value.Entry.Content, &aStruct)
		if aStruct.A != want {
			t.Errorf("watch returned: %v, want %v", aStruct.A, want)
		}
	case <-time.After(3 * time.Second):
		t.Error("failed to watch")
	}
}

func TestWatcher_convertingValueFunc(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	expectedLastKnownRevision := 1
	handler := func(w http.ResponseWriter, r *http.Request) {

		// Let's pretend that the content is modified after 100 millisecond and the revision is increased by 1.
		time.Sleep(100 * time.Millisecond)
		expectedLastKnownRevision++

		fmt.Fprint(w, `{"revision":`+strconv.Itoa(expectedLastKnownRevision)+`,
"entry":{"path":"/a.json", "type":"JSON", "content": {"a":`+strconv.Itoa(expectedLastKnownRevision)+`}}
}`)
	}

	mux.HandleFunc("/api/v1/projects/foo/repos/bar/contents/a.json", handler)

	query := &Query{Path: "/a.json", Type: Identity}
	fw, _ := c.FileWatcher("foo", "bar", query)

	myCh := make(chan WatchResult, 128)
	listener := func(value WatchResult) { myCh <- value }
	_ = fw.Watch(listener)

	want := 2
	for i := 0; i < 10; i++ {
		select {
		case value := <-myCh:
			aStruct := struct {
				A int `json:"a"`
			}{}
			_ = json.Unmarshal(value.Entry.Content, &aStruct)
			if aStruct.A != want {
				t.Errorf("watch returned: %v, want %v", aStruct.A, want)
			}
		case <-time.After(3 * time.Second):
			t.Error("failed to watch")
		}
		want++
	}
	fw.Close()
}

func TestWatcher_closed_AwaitInitialValue(t *testing.T) {
	c, _, teardown := setup()
	defer teardown()

	query := &Query{Path: "/a.json", Type: Identity}
	fw, _ := c.watch.fileWatcher(context.Background(), "foo", "bar", query)

	latest := fw.Latest()
	want := "latest is not set yet"
	if latest.Err.Error() != want {
		t.Errorf("latest: %+v, want %+v", latest.Err, want)
	}

	done := make(chan struct{})
	go func() {
		latest := fw.AwaitInitialValue()
		want := "watcher is closed"
		if latest.Err.Error() != want {
			t.Errorf("latest from AwaitInitialValue: %+v, want %+v", latest.Err, want)
		}
		done <- struct{}{}
	}()
	fw.Close()
	<-done
}

func TestWatcher_started_AwaitInitialValue(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	mux.HandleFunc("/api/v1/projects/foo/repos/bar/contents/a.json",
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, response)
		})

	query := &Query{Path: "/a.json", Type: Identity}
	fw, _ := c.watch.fileWatcher(context.Background(), "foo", "bar", query)
	defer fw.Close()

	done := make(chan struct{})
	go func() {
		latest := fw.AwaitInitialValue()
		if latest == nil {
			panic("latest from AwaitInitialValue is not valid")
		}

		want := 3
		if latest.Revision != want {
			t.Errorf("latest from AwaitInitialValue: %+v, want %+v", latest.Revision, want)
		}

		latest2 := fw.Latest()
		if !reflect.DeepEqual(latest2.Entry, latest.Entry) ||
			latest2.HttpStatusCode != latest.HttpStatusCode ||
			latest2.Revision != latest.Revision {
			t.Errorf("latest: %+v, want %+v", latest2, latest)
		}

		done <- struct{}{}
	}()
	fw.start()
	<-done
}

func TestRepoWatcher(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	expectedLastKnownRevision := 1
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Let's pretend that the content is modified after 100 millisecond and the revision is increased by 1.
		time.Sleep(100 * time.Millisecond)
		expectedLastKnownRevision++

		fmt.Fprint(w, `{"revision":`+strconv.Itoa(expectedLastKnownRevision)+`}`)
	}

	mux.HandleFunc("/api/v1/projects/foo/repos/bar/contents/**", handler)

	fw, _ := c.RepoWatcher("foo", "bar", "/**")
	defer fw.Close()

	myCh := make(chan int, 128)
	listener := func(value WatchResult) { myCh <- value.Revision }
	_ = fw.Watch(listener)

	want := 2
	for i := 0; i < 5; i++ {
		select {
		case revision := <-myCh:
			if revision != want {
				t.Errorf("watch returned: %v, want %v", revision, want)
			}
		case <-time.After(3 * time.Second):
			t.Error("failed to watch")
		}
		want++
	}
}

func TestRepoWatcherInvalidPathPattern(t *testing.T) {
	c, mux, teardown := setup()
	defer teardown()

	expectedLastKnownRevision := 1
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Let's pretend that the content is modified after 100 millisecond and the revision is increased by 1.
		time.Sleep(100 * time.Millisecond)
		expectedLastKnownRevision++

		fmt.Fprint(w, `{"revision":`+strconv.Itoa(expectedLastKnownRevision)+`}`)
	}

	mux.HandleFunc("/api/v1/projects/foo/repos/bar/contents/", handler)

	patterns := []string{"", "**", "**/a.json", "a.json"}
	want := 2
	for _, pattern := range patterns {
		fw, _ := c.RepoWatcher("foo", "bar", pattern)

		myCh := make(chan int, 128)
		listener := func(value WatchResult) { myCh <- value.Revision }
		_ = fw.Watch(listener)

		for i := 0; i < 2; i++ {
			select {
			case revision := <-myCh:
				if revision != want {
					t.Errorf("watch returned: %v, want %v", revision, want)
				}
			case <-time.After(3 * time.Second):
				t.Error("failed to watch")
			}
			want++
		}
		fw.Close()
		close(myCh)
	}
}
