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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"

	"github.com/urfave/cli/v2"
	"go.linecorp.com/centraldogma"
)

type commitType int

const (
	_ commitType = iota
	addition
	edition
	removal
)

var tempFileName = "commit-message.txt"

func getCommitMessage(c *cli.Context, out io.Writer, filePath string, commitType commitType) (*centraldogma.CommitMessage, error) {
	message := c.String("message")
	if len(message) != 0 {
		return &centraldogma.CommitMessage{Summary: message}, nil
	}

	tempFilePath, fd, err := newTempFile(tempFileName)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempFilePath)
	defer fd.Close()

	tmpl, _ := template.New("").Parse(commitMessageTemplate)
	tmpl.Execute(fd, typeWithFile{FilePath: filePath, CommitType: commitType})

	cmd := cmdToOpenEditor(out, tempFilePath)
	if err = cmd.Start(); err != nil {
		// Failed to launch the editor.
		return messageFromCLI()
	}

	err = cmd.Wait()
	if err != nil {
		return nil, errors.New("failed to write the commit message")
	}
	return messageFrom(tempFilePath)
}

func messageFromCLI() (*centraldogma.CommitMessage, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter commit message: ")
	summary, _ := reader.ReadString('\n')
	if len(summary) == 0 {
		return nil, errors.New("you must input commit message")
	}
	commitMessage := &centraldogma.CommitMessage{Summary: summary}

	fmt.Print("\nEnter detail: ")
	detail, _ := reader.ReadString('\n')
	if len(detail) != 0 {
		commitMessage.Detail = detail
		commitMessage.Markup = "PLAINTEXT"
	}

	return commitMessage, nil
}

func messageFrom(filePath string) (*centraldogma.CommitMessage, error) {
	fd, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	scanner := bufio.NewScanner(fd)

	summary, err := getSummary(scanner)
	if err != nil {
		return nil, err
	}
	commitMessage := &centraldogma.CommitMessage{Summary: summary}

	detail := getDetail(scanner)
	if len(detail) != 0 {
		commitMessage.Detail = detail
		commitMessage.Markup = "PLAINTEXT"
	}

	return commitMessage, nil
}

func getSummary(scanner *bufio.Scanner) (string, error) {
	line := ""
	for scanner.Scan() {
		line = strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "#") && len(line) != 0 {
			return line, nil
		}
	}
	return "", errors.New("must input the summary")
}

func getDetail(scanner *bufio.Scanner) string {
	var buf bytes.Buffer
	passedEmptyLinesUnderSummary := false
	for scanner.Scan() {
		line := strings.TrimRightFunc(scanner.Text(), unicode.IsSpace)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) == 0 && !passedEmptyLinesUnderSummary {
			continue
		} else {
			passedEmptyLinesUnderSummary = true
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	// remove trailing empty lines
	regex, _ := regexp.Compile(`\n{2,}\z`)
	return regex.ReplaceAllString(buf.String(), "")
}

type typeWithFile struct {
	FilePath   string
	CommitType commitType
}

var commitMessageTemplate = `
# Please enter the commit message for your changes. Lines starting\n
# with '#' will be ignored, and an empty message aborts the commit.\n
#
# Changes to be committed:
#   {{if eq .commitType 1}}new file{{else if eq .commitType 2}}modified{{else}}deleted{{end}}: {{.FilePath}}
#
`

func cmdToOpenEditor(out io.Writer, filePath string) *exec.Cmd {
	editor := editor()
	cmd := exec.Command(editor, filePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	return cmd
}

func editor() string {
	editor := os.Getenv("EDITOR")
	if len(editor) != 0 {
		return editor
	}
	out, err := exec.Command("git", "var", "GIT_EDITOR").Output()
	if err == nil {
		editor = strings.TrimSpace(string(out))
		if len(editor) != 0 {
			return editor
		}
	}
	if strings.EqualFold(runtime.GOOS, "windows") {
		return "start"
	} else {
		return "vim"
	}
}

func newTempFile(tempFileName string) (string, *os.File, error) {
	// TODO(minwoox) change to a file in a fixed place like git
	tempFilePath := os.TempDir() + string(filepath.Separator) + tempFileName
	fd, err := os.Create(tempFilePath)
	if err != nil {
		return "", nil, errors.New("failed to create a temp file")
	}
	return tempFilePath, fd, nil
}

func putIntoTempFile(entry *centraldogma.Entry) (string, error) {
	tempFilePath, fd, err := newTempFile(path.Base(entry.Path))
	if err != nil {
		return "", err
	}
	defer fd.Close()
	if entry.Type == centraldogma.JSON {
		b := safeMarshalIndent(entry.Content)
		if _, err := fd.Write(b); err != nil {
			return "", err
		}
		return tempFilePath, nil
	} else if entry.Type == centraldogma.Text {
		_, err = fd.Write(entry.Content)
		if err != nil {
			return "", err
		}
	}
	return tempFilePath, nil
}

func newUpsertChangeFromFile(fileName, repositoryPath string) (*centraldogma.Change, error) {
	fileInfo, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s does not exist", fileName)
	}
	if fileInfo.IsDir() {
		return nil, fmt.Errorf("%s is a directory", fileName)
	}

	change := &centraldogma.Change{Path: repositoryPath}
	buf, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(strings.ToLower(fileName), ".json") {
		change.Type = centraldogma.UpsertJSON
		if !json.Valid(buf) {
			return nil, fmt.Errorf("not a valid JSON file: %s", fileName)
		}
		var temp interface{}
		err = json.Unmarshal(buf, &temp)
		if err != nil {
			return nil, err
		}
		change.Content = temp
	} else {
		change.Content = string(buf)
		change.Type = centraldogma.UpsertText
	}
	return change, nil
}

func marshalIndentObject(data interface{}) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

func safeMarshalIndent(src []byte) []byte {
	if json.Valid(src) {
		dst := new(bytes.Buffer)
		_ = json.Indent(dst, src, "", "  ")
		return dst.Bytes()
	}
	return src
}

type dogmaClientCtxKey struct{}

var dogmaClientCtxKeyInstance = &dogmaClientCtxKey{}

func getDogmaClientFrom(ctx context.Context) *centraldogma.Client {
	v, ok := ctx.Value(dogmaClientCtxKeyInstance).(*centraldogma.Client)
	if !ok {
		return nil
	}
	return v
}

func putDogmaClientTo(ctx context.Context, client *centraldogma.Client) context.Context {
	return context.WithValue(ctx, dogmaClientCtxKeyInstance, client)
}
