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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"

	dogma "go.linecorp.com/centraldogma"

	"github.com/urfave/cli"
)

type watchCommand struct {
	repo      repositoryRequestInfo
	jsonPaths []string
	streaming bool
	listener  string
}

type listenerExecError struct {
	underlying error
	command    string
}

func (e *listenerExecError) Error() string {
	return fmt.Sprintf("failed to execute listener %s: %s", e.command, e.underlying.Error())
}

func (wc *watchCommand) defaultListener(watchResult dogma.WatchResult) error {
	repo := wc.repo
	fmt.Printf("Watcher noticed updated file: %s/%s%s, rev=%v\n",
		repo.projName, repo.repoName, repo.path, watchResult.Revision)
	content := ""
	if strings.HasSuffix(strings.ToLower(repo.path), ".json") {
		content = string(safeMarshalIndent(watchResult.Entry.Content))
	} else {
		content = string(watchResult.Entry.Content)
	}
	fmt.Printf("Content:\n%s\n", content)
	return nil
}

func (wc *watchCommand) commandExecutionListener(watchResult dogma.WatchResult) error {
	command := exec.Command(wc.listener)
	command.Env = append(os.Environ(),
		"DOGMA_WATCH_EVENT_PATH="+watchResult.Entry.Path,
		"DOGMA_WATCH_EVENT_CONTENT_TYPE="+watchResult.Entry.Type.String(),
		"DOGMA_WATCH_EVENT_REV="+strconv.Itoa(watchResult.Revision),
		"DOGMA_WATCH_EVENT_URL="+watchResult.Entry.URL)
	command.Stdin = bytes.NewReader(watchResult.Entry.Content)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	err := command.Run()
	if err != nil {
		return &listenerExecError{underlying: err, command: wc.listener}
	}
	return nil
}

func (wc *watchCommand) execute(c *cli.Context) error {
	client, err := newDogmaClient(c, wc.repo.remoteURL)
	if err != nil {
		return err
	}
	return wc.executeWithDogmaClient(c, client)
}

func (wc *watchCommand) executeWithDogmaClient(c *cli.Context, client *dogma.Client) error {
	repo := wc.repo

	normalizedRevision, _, err := client.NormalizeRevision(
		context.Background(), repo.projName, repo.repoName, repo.revision)
	if err != nil {
		return err
	}

	query := createQuery(repo.path, wc.jsonPaths)
	fw, err := client.FileWatcher(repo.projName, repo.repoName, query)
	if err != nil {
		return err
	}

	// prepare context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 2)
	notifyDone := func(err error) {
		select {
		case <-ctx.Done():
		case done <- err: // notify
		}
	}

	listener := wc.defaultListener

	if wc.listener != "" {
		listener = wc.commandExecutionListener
	}

	// start watching
	err = fw.Watch(func(watchResult dogma.WatchResult) {
		if watchResult.Revision > normalizedRevision {
			err := listener(watchResult)
			if err != nil || !wc.streaming {
				fw.Close()
				notifyDone(err)
			}
		}
	})
	if err != nil {
		fw.Close()
		return err
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		select {
		case <-ctx.Done():
			return

		case <-signalChan:
			fmt.Println("\nReceived an interrupt, stopping watcher...")
			fw.Close()
			notifyDone(nil)
		}
	}()

	// wait until notified to done channel
	return <-done
}

// newWatchCommand creates the watchCommand.
func newWatchCommand(c *cli.Context) (Command, error) {
	repo, err := newRepositoryRequestInfo(c)
	if err != nil {
		return nil, err
	}

	return &watchCommand{repo: repo, jsonPaths: c.StringSlice("jsonpath"), streaming: c.Bool("streaming"), listener: c.String("listener")}, nil
}
