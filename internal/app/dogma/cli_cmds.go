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
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli"
)

var commitMessageFlag = cli.StringFlag{
	Name:  "message, m",
	Usage: "Specifies the commit message",
}

var revisionFlag = cli.StringFlag{
	Name:  "revision, r",
	Usage: "Specifies the revision to operate",
}

var recursiveFlag = cli.BoolFlag{
	Name:  "recursive",
	Usage: "Specifies whether to download a whole directory",
}

var jsonPathFlag = cli.StringSliceFlag{
	Name:  "jsonpath, j",
	Usage: "Specifies the JSON path expressions to apply",
}

var fromRevisionFlag = cli.StringFlag{
	Name:  "from",
	Usage: "Specifies the revision to apply from",
}

var toRevisionFlag = cli.StringFlag{
	Name:  "to",
	Usage: "Specifies the revision to apply until",
}

var maxCommitsFlag = cli.IntFlag{
	Name:  "max-commits",
	Usage: "Specifies the number of maximum commits to fetch",
}

var streamingFlag = cli.BoolFlag{
	Name:  "streaming, s",
	Usage: "Specifies whether to keep watching the file",
}

var listenerFlag = cli.StringFlag{
	Name:  "listener, l",
	Usage: "Specifies the `executable` path that handles watch events",
}

var printFormatFlags = []cli.Flag{
	cli.BoolFlag{
		Name:   "pretty",
		Hidden: true,
	},
	cli.BoolFlag{
		Name:   "simple",
		Hidden: true,
	},
	cli.BoolFlag{
		Name:   "json",
		Hidden: true,
	},
}

type PrintStyle int

const (
	_ PrintStyle = iota
	Pretty
	Simple
	JSON
)

func getPrintStyle(c *cli.Context) (PrintStyle, error) {
	var ps PrintStyle
	if c.Bool("pretty") {
		ps = Pretty
	}
	if c.Bool("simple") {
		if ps != 0 {
			return 0, fmt.Errorf("duplicate print style (pretty: %t, simple: %t, json: %t)",
				c.Bool("pretty"), c.Bool("simple"), c.Bool("json"))
		}
		ps = Simple
	}
	if c.Bool("json") {
		if ps != 0 {
			return 0, fmt.Errorf("duplicate print style (pretty: %t, simple: %t, json: %t)",
				c.Bool("pretty"), c.Bool("simple"), c.Bool("json"))
		}
		ps = JSON
	}
	if ps == 0 {
		ps = Pretty
	}
	return ps, nil
}

func printWithStyle(out io.Writer, data interface{}, format PrintStyle) {
	// TODO implement this method
	buf, _ := marshalIndentObject(data)
	fmt.Fprintf(out, "%s\n", buf)
}

func newCommandLineError(c *cli.Context) *cli.ExitError {
	com := c.Command
	return cli.NewExitError("usage: "+com.Name+" "+com.ArgsUsage, 1)
}

func CLICommands() []cli.Command {
	return []cli.Command{
		{
			Name:      "ls",
			Usage:     "Lists the projects, repositories or files",
			ArgsUsage: "[<project_name>[/<repository_name>[/<path>]]]",
			Flags:     append(printFormatFlags, revisionFlag),
			Action: func(c *cli.Context) error {
				style, err := getPrintStyle(c)
				if err != nil {
					return err
				}
				command, err := newLSCommand(c, os.Stdout, style)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:      "new",
			Usage:     "Creates a project or repository",
			ArgsUsage: "<project_name>[/<repository_name>]",
			Action: func(c *cli.Context) error {
				command, err := newNewCommand(c, os.Stdout)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:      "put",
			Usage:     "Puts a file to the repository",
			ArgsUsage: "<project_name>/<repository_name>[/<path>] file_path",
			Flags:     []cli.Flag{revisionFlag, commitMessageFlag},
			Action: func(c *cli.Context) error {
				command, err := newPutCommand(c, os.Stdout)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:      "edit",
			Usage:     "Edits a file in the path",
			ArgsUsage: "<project_name>/<repository_name>/<path>",
			Flags:     []cli.Flag{revisionFlag, commitMessageFlag},
			Action: func(c *cli.Context) error {
				command, err := newEditCommand(c, os.Stdout)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:      "get",
			Usage:     "Downloads a file in the path",
			ArgsUsage: "<project_name>/<repository_name>/<path>",
			Flags:     []cli.Flag{revisionFlag, jsonPathFlag, recursiveFlag},
			Action: func(c *cli.Context) error {
				command, err := newGetCommand(c, os.Stdout)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:      "cat",
			Usage:     "Prints a file in the path",
			ArgsUsage: "<project_name>/<repository_name>/<path>",
			Flags:     []cli.Flag{revisionFlag, jsonPathFlag},
			Action: func(c *cli.Context) error {
				command, err := newCatCommand(c, os.Stdout)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:  "watch",
			Usage: "Watches a file in the path",
			Description: `Watch events are printed to stdout by default.
   You can customize this behavior by using --listener <executable> option.
   The command you specified as <executable> can read updated content body through its STDIN.
   Other meta data about the watch event are available via environment variables below.

     DOGMA_WATCH_EVENT_PATH - The path to the file you're watching
     DOGMA_WATCH_EVENT_CONTENT_TYPE - The content type of the file, JSON or TEXT
     DOGMA_WATCH_EVENT_REV - The revision number of the watch event
     DOGMA_WATCH_EVENT_URL - The URL of the target file

   e.g.
     # Print foo.json content when it gets updated
     dogma watch --listener cat /pj/repo/foo.json`,
			ArgsUsage: "<project_name>/<repository_name>/<path>",
			Flags:     []cli.Flag{revisionFlag, jsonPathFlag, streamingFlag, listenerFlag},
			Action: func(c *cli.Context) error {
				command, err := newWatchCommand(c, os.Stdout)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:      "rm",
			Usage:     "Removes a file in the path",
			ArgsUsage: "<project_name>/<repository_name>/<path>",
			Flags:     []cli.Flag{revisionFlag, commitMessageFlag},
			Action: func(c *cli.Context) error {
				command, err := newRMCommand(c, os.Stdout)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:      "diff",
			Usage:     "Gets diff of given path",
			ArgsUsage: "<project_name>/<repository_name>[/<path>]",
			Flags:     append(printFormatFlags, fromRevisionFlag, toRevisionFlag),
			Action: func(c *cli.Context) error {
				style, err := getPrintStyle(c)
				if err != nil {
					return err
				}
				command, err := newDiffCommand(c, os.Stdout, style)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:      "log",
			Usage:     "Shows commit logs of the path",
			ArgsUsage: "<project_name>/<repository_name>[/<path>]",
			Flags:     append(printFormatFlags, fromRevisionFlag, toRevisionFlag, maxCommitsFlag),
			Action: func(c *cli.Context) error {
				style, err := getPrintStyle(c)
				if err != nil {
					return err
				}
				command, err := newLogCommand(c, os.Stdout, style)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
		{
			Name:      "normalize",
			Usage:     "Normalizes a revision into an absolute revision",
			ArgsUsage: "<project_name>/<repository_name>",
			Flags:     []cli.Flag{revisionFlag},
			Action: func(c *cli.Context) error {
				command, err := newNormalizeCommand(c, os.Stdout)
				if err != nil {
					return newCommandLineError(c)
				}
				err = command.execute(c)
				if err != nil {
					return cli.NewExitError(err, 1)
				}
				return nil
			},
		},
	}
}
