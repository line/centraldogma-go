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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type watchService service

// WatchResult represents a result from watch operation.
type WatchResult struct {
	Commit *Commit
	Entry  *Entry
	Res    *http.Response
	Err    error
}

type commitWithEntry struct {
	*Commit
	Entry *Entry `json:"entry,omitempty"`
}

func (ws *watchService) watchFile(ctx context.Context, projectName, repoName, lastKnownRevision string,
	query *Query, timeout time.Duration) <-chan *WatchResult {

	// initialize watch result channel
	watchResult := make(chan *WatchResult, 1)

	// validate query
	if query == nil {
		watchResult <- &WatchResult{Err: ErrQueryMustBeSet}
		return watchResult
	}

	// Normalize query path when it does not start with "/".
	if len(query.Path) != 0 && !strings.HasPrefix(query.Path, "/") {
		query.Path = "/" + query.Path
	}

	u := fmt.Sprintf("%vprojects/%v/repos/%v/contents%v", defaultPathPrefix, projectName, repoName, query.Path)
	v := &url.Values{}
	if query != nil && query.Type == JSONPath {
		if err := setJSONPaths(v, query.Path, query.Expressions); err != nil {
			watchResult <- &WatchResult{Err: err}
			return watchResult
		}
	}
	u += encodeValues(v)
	ws.watchRequest(ctx, watchResult, u, lastKnownRevision, timeout)
	return watchResult
}

func (ws *watchService) watchRepo(ctx context.Context,
	projectName, repoName, lastKnownRevision, pathPattern string, timeout time.Duration) <-chan *WatchResult {

	// initialize watch result channel
	watchResult := make(chan *WatchResult, 1)

	// Normalize pathPattern
	if len(pathPattern) == 0 {
		pathPattern = "/**"
	} else if strings.HasPrefix(pathPattern, "**") {
		pathPattern = "/" + pathPattern
	} else if !strings.HasPrefix(pathPattern, "/") {
		pathPattern = "/**/" + pathPattern
	}

	u := fmt.Sprintf("%vprojects/%v/repos/%v/contents%v", defaultPathPrefix, projectName, repoName, pathPattern)
	ws.watchRequest(ctx, watchResult, u, lastKnownRevision, timeout)
	return watchResult
}

func (ws *watchService) watchRequest(ctx context.Context, watchResult chan<- *WatchResult,
	u, lastKnownRevision string, timeout time.Duration) {

	// initialize request
	req, err := ws.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		watchResult <- &WatchResult{Err: err}
		return
	}
	if len(lastKnownRevision) != 0 {
		req.Header.Set("if-none-match", lastKnownRevision)
	} else {
		req.Header.Set("if-none-match", "-1")
	}
	if timeout != 0 {
		req.Header.Set("prefer", fmt.Sprintf("wait=%v", timeout.Seconds()))
	}

	// TODO(linxGnu): worker pool for this task
	go func() {
		reqCtx, cancel := context.WithTimeout(ctx, timeout+time.Second) // wait more than server

		commitWithEntry := new(commitWithEntry)
		res, err := ws.client.do(reqCtx, req, commitWithEntry)
		if err != nil {
			if err == context.DeadlineExceeded {
				watchResult <- &WatchResult{Res: res, Err: fmt.Errorf("watch request timeout: %.3f second(s)", timeout.Seconds())}
			} else {
				watchResult <- &WatchResult{Res: res, Err: err}
			}
		} else {
			watchResult <- &WatchResult{Commit: commitWithEntry.Commit, Entry: commitWithEntry.Entry,
				Res: res, Err: nil}
		}

		cancel()
	}()
}

const watchTimeout = 1 * time.Minute

// These constants represent the state of a watcher.
const (
	initial int32 = iota
	started
	stopped
)

// WatchListener listens to Watcher.
type WatchListener func(revision int, value interface{})

// Watcher watches the changes of a repository or a file.
type Watcher struct {
	state int32

	initialValueCh      chan *Latest // channel whose buffer is 1.
	isInitialValueChSet int32        // 0 is false, 1 is true

	watchCTX        context.Context
	watchCancelFunc func()

	latest              atomic.Value
	updateListenerChans []chan *Latest
	listenersMutex      sync.RWMutex

	doWatchFunc          func(lastKnownRevision int) <-chan *WatchResult
	convertingResultFunc func(result *WatchResult) *Latest

	projectName string
	repoName    string
	pathPattern string

	numAttemptsSoFar int
}

// Latest represents a holder of the latest known value and its Revision retrieved by Watcher.
type Latest struct {
	Revision int
	Value    interface{}
	Err      error
}

func newWatcher(projectName, repoName, pathPattern string) *Watcher {
	watchCTX, watchCancelFunc := context.WithCancel(context.Background())
	return &Watcher{
		state:           initial,
		initialValueCh:  make(chan *Latest, 1),
		watchCTX:        watchCTX,
		watchCancelFunc: watchCancelFunc,
		projectName:     projectName,
		repoName:        repoName,
		pathPattern:     pathPattern,
	}
}

// AwaitInitialValue awaits for the initial value to be available.
func (w *Watcher) AwaitInitialValue() Latest {
	latest := <-w.initialValueCh
	// Put it back to the channel so that this can return the value multiple times.
	w.initialValueCh <- latest
	return *latest
}

// AwaitInitialValueWith awaits for the initial value to be available during the specified timeout.
func (w *Watcher) AwaitInitialValueWith(timeout time.Duration) Latest {
	select {
	case latest := <-w.initialValueCh:
		// Put it back to the channel so that this can return the value multiple times.
		w.initialValueCh <- latest
		return *latest
	case <-time.After(timeout):
		return Latest{Err: fmt.Errorf("failed to get the initial value. timeout: %v", timeout)}
	}
}

func (w *Watcher) getLatest() (lt *Latest) {
	loaded := w.latest.Load()
	if loaded != nil {
		lt, _ = loaded.(*Latest)
	}
	return
}

// Latest returns the latest Revision and value of WatchFile() or WatchRepository() result.
func (w *Watcher) Latest() Latest {
	latest := w.getLatest()
	if latest != nil {
		return *latest
	}
	return Latest{Err: ErrLatestNotSet}
}

// LatestValue returns the latest value of watchFile() or WatchRepository() result.
func (w *Watcher) LatestValue() (interface{}, error) {
	latest := w.getLatest()
	if latest != nil {
		return latest.Value, latest.Err
	}
	return nil, ErrLatestNotSet
}

// LatestValueOr returns the latest value of watchFile() or WatchRepository() result. If it's not available, this
// returns the defaultValue.
func (w *Watcher) LatestValueOr(defaultValue interface{}) interface{} {
	latest := w.Latest()
	if latest.Err != nil {
		return defaultValue
	}
	return latest.Value
}

// Close stops watching the file specified in the Query or the pathPattern in the repository.
func (w *Watcher) Close() {
	atomic.StoreInt32(&w.state, stopped)
	latest := &Latest{Err: ErrWatcherClosed}
	if atomic.CompareAndSwapInt32(&w.isInitialValueChSet, 0, 1) {
		// The initial latest was not set before. So write the value to initialValueCh as well.
		w.initialValueCh <- latest
	}
	w.watchCancelFunc() // After the first call, subsequent calls to a CancelFunc do nothing.
}

// Watch registers a func that will be invoked when the value of the watched entry becomes available or changes.
func (w *Watcher) Watch(listener WatchListener) error {
	if listener == nil {
		return nil // do nothing
	}

	// check watcher is stopped
	if w.isStopped() {
		return ErrWatcherClosed
	}

	// start notifier which notify on update
	ch := make(chan *Latest, 32)
	go w.notifier(listener, ch)

	w.listenersMutex.Lock()
	w.updateListenerChans = append(w.updateListenerChans, ch)
	w.listenersMutex.Unlock()

	if latest := w.Latest(); latest.Err == nil {
		select {
		case <-w.watchCTX.Done():
			return w.watchCTX.Err()

		case ch <- &latest:
		}
	}

	return nil
}

func (ws *watchService) fileWatcher(projectName, repoName string, query *Query) (*Watcher, error) {
	if query == nil {
		return nil, ErrQueryMustBeSet
	}

	w := newWatcher(projectName, repoName, query.Path)
	w.doWatchFunc = func(lastKnownRevision int) <-chan *WatchResult {
		return ws.watchFile(w.watchCTX, projectName, repoName, strconv.Itoa(lastKnownRevision),
			query, watchTimeout)
	}
	w.convertingResultFunc = func(result *WatchResult) *Latest {
		value := result.Entry.Content
		return &Latest{Revision: result.Commit.Revision, Value: value}
	}
	return w, nil
}

func (ws *watchService) repoWatcher(projectName, repoName, pathPattern string) (*Watcher, error) {
	w := newWatcher(projectName, repoName, pathPattern)
	w.doWatchFunc = func(lastKnownRevision int) <-chan *WatchResult {
		return ws.watchRepo(w.watchCTX, projectName, repoName, strconv.Itoa(lastKnownRevision),
			pathPattern, watchTimeout)
	}
	w.convertingResultFunc = func(result *WatchResult) *Latest {
		revision := result.Commit.Revision
		return &Latest{Revision: revision, Value: revision}
	}
	return w, nil
}

func (w *Watcher) start() {
	if atomic.CompareAndSwapInt32(&w.state, initial, started) {
		go w.scheduleWatch()
	}
}

func (w *Watcher) isStopped() bool {
	state := atomic.LoadInt32(&w.state)
	return state == stopped
}

func (w *Watcher) scheduleWatch() {
	if w.isStopped() {
		return
	}

	for {
		select {
		case <-w.watchCTX.Done():
			return

		default:
			w.doWatch()
		}
	}
}

func (w *Watcher) doWatch() {
	if w.isStopped() {
		return
	}

	var lastKnownRevision int
	curLatest := w.getLatest()
	if curLatest == nil || curLatest.Revision == 0 {
		lastKnownRevision = 1 // Init revision
	} else {
		lastKnownRevision = curLatest.Revision
	}

	select {

	case <-w.watchCTX.Done():
		return

	case watchResult := <-w.doWatchFunc(lastKnownRevision):
		if watchResult.Err != nil {
			if watchResult.Err == context.Canceled {
				// Cancelled by close()
				return
			}

			log.Debug(watchResult.Err)

			// wait for next attempt
			w.numAttemptsSoFar++
			w.delay()
			return
		}

		newLatest := w.convertingResultFunc(watchResult)
		if w.isInitialValueChSet == 0 && atomic.CompareAndSwapInt32(&w.isInitialValueChSet, 0, 1) {
			// The initial latest is set for the first time. So write the value to initialValueCh as well.
			w.initialValueCh <- newLatest
		}

		// store latest
		w.latest.Store(newLatest)

		// log latest revision
		log.Debugf("Watcher noticed updated file: %s/%s%s, rev=%v",
			w.projectName, w.repoName, w.pathPattern, newLatest.Revision)

		// notify listener
		w.notifyListeners()

		// wait for next attempt
		w.numAttemptsSoFar = 0
		w.delay()
	}
}

func (w *Watcher) delay() {
	var delay time.Duration

	if w.numAttemptsSoFar == 0 {
		delay = delayOnSuccess
	} else {
		delay = nextDelay(w.numAttemptsSoFar)
	}

	if delay > 0 {
		select {
		case <-w.watchCTX.Done():
		case <-time.After(delay):
		}
	}
}

func (w *Watcher) notifyListeners() {
	if w.isStopped() {
		// Do not notify after stopped.
		return
	}

	latest := w.Latest()

	w.listenersMutex.RLock()
	listenerChanSnapshot := make([]chan *Latest, len(w.updateListenerChans))
	copy(listenerChanSnapshot, w.updateListenerChans)
	w.listenersMutex.RUnlock()

	for _, listener := range listenerChanSnapshot {
		select {
		case <-w.watchCTX.Done():
			return

		case listener <- &latest:
		}
	}
}

func (w *Watcher) notifier(listener WatchListener, ch <-chan *Latest) {
	for {
		select {
		case <-w.watchCTX.Done():
			return

		case latest, ok := <-ch:
			if !ok { // channel is closed
				return
			}

			if latest != nil {
				listener(latest.Revision, latest.Value)
			}
		}
	}
}
