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

const timeoutBuffer = 5 * time.Second

type watchService service

// WatchResult represents a result from watch operation.
type WatchResult struct {
	Revision       int   `json:"revision"`
	Entry          Entry `json:"entry,omitempty"`
	HttpStatusCode int
	Err            error
}

func (ws *watchService) watchFile(
	ctx context.Context,
	projectName, repoName, lastKnownRevision string,
	query *Query,
	timeout time.Duration,
) *WatchResult {

	// validate query
	if query == nil {
		return &WatchResult{Err: ErrQueryMustBeSet}
	}

	// Normalize query path when it does not start with "/".
	if len(query.Path) != 0 && !strings.HasPrefix(query.Path, "/") {
		query.Path = "/" + query.Path
	}

	u := fmt.Sprintf("%vprojects/%v/repos/%v/contents%v", defaultPathPrefix, projectName, repoName, query.Path)
	v := &url.Values{}
	if query != nil && query.Type == JSONPath {
		if err := setJSONPaths(v, query.Path, query.Expressions); err != nil {
			return &WatchResult{Err: err}
		}
	}
	u += encodeValues(v)

	return ws.watchRequest(ctx, u, lastKnownRevision, timeout)
}

func (ws *watchService) watchRepo(
	ctx context.Context,
	projectName, repoName, lastKnownRevision,
	pathPattern string,
	timeout time.Duration,
) *WatchResult {

	// Normalize pathPattern
	if len(pathPattern) == 0 {
		pathPattern = "/**"
	} else if strings.HasPrefix(pathPattern, "**") {
		pathPattern = "/" + pathPattern
	} else if !strings.HasPrefix(pathPattern, "/") {
		pathPattern = "/**/" + pathPattern
	}

	u := fmt.Sprintf("%vprojects/%v/repos/%v/contents%v", defaultPathPrefix, projectName, repoName, pathPattern)

	return ws.watchRequest(ctx, u, lastKnownRevision, timeout)
}

func (ws *watchService) watchRequest(
	ctx context.Context,
	u, lastKnownRevision string,
	timeout time.Duration,
) *WatchResult {

	// initialize request
	req, err := ws.client.newRequest(http.MethodGet, u, nil)
	if err != nil {
		return &WatchResult{Err: err}
	}
	if len(lastKnownRevision) != 0 {
		req.Header.Set("if-none-match", lastKnownRevision)
	} else {
		req.Header.Set("if-none-match", "-1")
	}
	if timeout != 0 {
		req.Header.Set("prefer", fmt.Sprintf("wait=%v", timeout.Seconds()))
	}

	// create new request context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, timeout+timeoutBuffer) // wait more than server
	defer cancel()

	watchResult := new(WatchResult)
	httpStatusCode, err := ws.client.do(reqCtx, req, watchResult)
	if err != nil {
		if err == context.DeadlineExceeded {
			err = fmt.Errorf("watch request timeout: %.3f second(s)", timeout.Seconds())
		}
		return &WatchResult{HttpStatusCode: httpStatusCode, Err: err}
	}

	watchResult.HttpStatusCode = httpStatusCode
	return watchResult
}

const defaultWatchTimeout = 1 * time.Minute

// These constants represent the state of a watcher.
const (
	initial int32 = iota
	started
	stopped
)

// WatchListener listens to Watcher.
type WatchListener func(result WatchResult)

// Watcher watches the changes of a repository or a file.
type Watcher struct {
	state int32

	initialValueCh      chan *WatchResult // channel whose buffer is 1.
	isInitialValueChSet int32             // 0 is false, 1 is true

	watchCTX        context.Context
	watchCancelFunc func()

	latest              atomic.Value
	updateListenerChans []chan *WatchResult
	listenersMutex      sync.RWMutex

	doWatchFunc func(ctx context.Context, lastKnownRevision int) *WatchResult

	projectName string
	repoName    string
	pathPattern string

	numAttemptsSoFar int
}

func newWatcher(ctx context.Context, projectName, repoName, pathPattern string) *Watcher {
	watchCTX, watchCancelFunc := context.WithCancel(ctx)
	return &Watcher{
		state:           initial,
		initialValueCh:  make(chan *WatchResult, 1),
		watchCTX:        watchCTX,
		watchCancelFunc: watchCancelFunc,
		projectName:     projectName,
		repoName:        repoName,
		pathPattern:     pathPattern,
	}
}

// AwaitInitialValue awaits for the initial value to be available.
func (w *Watcher) AwaitInitialValue() *WatchResult {
	latest := <-w.initialValueCh
	// Put it back to the channel so that this can return the value multiple times.
	w.initialValueCh <- latest
	return latest
}

// AwaitInitialValueWith awaits for the initial value to be available during the specified timeout.
func (w *Watcher) AwaitInitialValueWith(timeout time.Duration) *WatchResult {
	select {
	case latest := <-w.initialValueCh:
		// Put it back to the channel so that this can return the value multiple times.
		w.initialValueCh <- latest
		return latest
	case <-time.After(timeout):
		return &WatchResult{Err: fmt.Errorf("failed to get the initial value. timeout: %v", timeout)}
	}
}

func (w *Watcher) getLatest() (lt *WatchResult) {
	loaded := w.latest.Load()
	if loaded != nil {
		lt, _ = loaded.(*WatchResult)
	}
	return
}

// Latest returns the latest Revision and value of WatchFile() or WatchRepository() result.
func (w *Watcher) Latest() *WatchResult {
	latest := w.getLatest()
	if latest != nil {
		return latest
	}
	return &WatchResult{Err: ErrLatestNotSet}
}

// Close stops watching the file specified in the Query or the pathPattern in the repository.
func (w *Watcher) Close() {
	atomic.StoreInt32(&w.state, stopped)
	latest := &WatchResult{Err: ErrWatcherClosed}
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
	ch := make(chan *WatchResult, 32)
	go w.notifier(listener, ch)

	w.listenersMutex.Lock()
	w.updateListenerChans = append(w.updateListenerChans, ch)
	w.listenersMutex.Unlock()

	if latest := w.Latest(); latest.Err == nil {
		select {
		case <-w.watchCTX.Done():
			return w.watchCTX.Err()

		case ch <- latest:
		}
	}

	return nil
}

func (ws *watchService) fileWatcher(
	ctx context.Context,
	projectName, repoName string, query *Query,
) (*Watcher, error) {
	return ws.fileWatcherWithTimeout(ctx, projectName, repoName, query, defaultWatchTimeout)
}

func (ws *watchService) fileWatcherWithTimeout(
	ctx context.Context,
	projectName, repoName string, query *Query,
	timeout time.Duration,
) (*Watcher, error) {
	if query == nil {
		return nil, ErrQueryMustBeSet
	}

	w := newWatcher(ctx, projectName, repoName, query.Path)
	w.doWatchFunc = func(ctx context.Context, lastKnownRevision int) *WatchResult {
		return ws.watchFile(ctx, projectName, repoName, strconv.Itoa(lastKnownRevision),
			query, timeout)
	}
	return w, nil
}

func (ws *watchService) repoWatcher(
	ctx context.Context,
	projectName, repoName, pathPattern string,
) (*Watcher, error) {
	return ws.repoWatcherWithTimeout(ctx, projectName, repoName, pathPattern, defaultWatchTimeout)
}

func (ws *watchService) repoWatcherWithTimeout(
	ctx context.Context,
	projectName, repoName, pathPattern string,
	timeout time.Duration,
) (*Watcher, error) {
	w := newWatcher(ctx, projectName, repoName, pathPattern)
	w.doWatchFunc = func(ctx context.Context, lastKnownRevision int) *WatchResult {
		return ws.watchRepo(ctx, projectName, repoName, strconv.Itoa(lastKnownRevision),
			pathPattern, timeout)
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

	// do watch with context
	watchResult := w.doWatchFunc(w.watchCTX, lastKnownRevision)
	if watchResult == nil {
		// wait for next attempt
		w.numAttemptsSoFar++
		w.delay()
		return
	}
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

	// converting watch result and feed back to initial value channel if needed
	if w.isInitialValueChSet == 0 && atomic.CompareAndSwapInt32(&w.isInitialValueChSet, 0, 1) {
		// The initial latest is set for the first time. So write the value to initialValueCh as well.
		w.initialValueCh <- watchResult
	}

	// store latest
	w.latest.Store(watchResult)

	// log latest revision
	log.Debugf("Watcher noticed updated file: %s/%s%s, rev=%v",
		w.projectName, w.repoName, w.pathPattern, watchResult.Revision)

	// notify listener
	w.notifyListeners()

	// wait for next attempt
	w.numAttemptsSoFar = 0
	w.delay()
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
	listenerChanSnapshot := make([]chan *WatchResult, len(w.updateListenerChans))
	copy(listenerChanSnapshot, w.updateListenerChans)
	w.listenersMutex.RUnlock()

	for _, listener := range listenerChanSnapshot {
		select {
		case <-w.watchCTX.Done():
			return

		case listener <- latest:
		}
	}
}

func (w *Watcher) notifier(listener WatchListener, ch <-chan *WatchResult) {
	for {
		select {
		case <-w.watchCTX.Done():
			return

		case latest, ok := <-ch:
			if !ok { // channel is closed
				return
			}

			if latest != nil {
				listener(*latest)
			}
		}
	}
}
