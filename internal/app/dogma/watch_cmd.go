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
	"os"
	"os/signal"
	"strings"

	dogma "go.linecorp.com/centraldogma"

	"github.com/urfave/cli"
)

type watchCommand struct {
	repo      repositoryRequestInfo
	jsonPaths []string
	streaming bool
}

func (wc *watchCommand) execute(c *cli.Context) error {
	repo := wc.repo
	client, err := newDogmaClient(c, repo.remoteURL)
	if err != nil {
		return err
	}

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

	done := make(chan struct{}, 2)
	notifyDone := func() {
		select {
		case <-ctx.Done():
		case done <- struct{}{}: // notify
		}
	}

	listener := func(watchResult dogma.WatchResult) {
		revision := watchResult.Revision
		if revision > normalizedRevision {
			fmt.Printf("Watcher noticed updated file: %s/%s%s, rev=%v\n",
				repo.projName, repo.repoName, repo.path, revision)
			content := ""
			if strings.HasSuffix(strings.ToLower(repo.path), ".json") {
				content = string(safeMarshalIndent(watchResult.Entry.Content))
			} else {
				content = string(watchResult.Entry.Content)
			}
			fmt.Printf("Content:\n%s\n", content)

			if !wc.streaming {
				fw.Close()
				notifyDone()
			}
		}
	}

	// start watching
	err = fw.Watch(listener)
	if err != nil {
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
			notifyDone()
		}
	}()

	// wait until notified to done channel
	<-done

	return nil
}

// newWatchCommand creates the watchCommand.
func newWatchCommand(c *cli.Context) (Command, error) {
	repo, err := newRepositoryRequestInfo(c)
	if err != nil {
		return nil, err
	}

	return &watchCommand{repo: repo, jsonPaths: c.StringSlice("jsonpath"), streaming: c.Bool("streaming")}, nil
}
