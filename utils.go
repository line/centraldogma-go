// Copyright 2019 LINE Corporation
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
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func setFromTo(v *url.Values, from, to string) {
	if len(from) != 0 {
		v.Set("from", from)
	}

	if len(to) != 0 {
		v.Set("to", to)
	}
}

func setRevision(v *url.Values, revision string) {
	if len(revision) != 0 {
		v.Set("revision", revision)
	}
}

func setPath(v *url.Values, path string) {
	if len(path) != 0 {
		v.Set("path", path)
	}
}

func setPathPattern(v *url.Values, pathPattern string) {
	if len(pathPattern) != 0 {
		v.Set("pathPattern", pathPattern)
	}
}

func setMaxCommits(v *url.Values, maxCommits int) {
	if maxCommits != 0 {
		v.Set("maxCommits", strconv.Itoa(maxCommits))
	}
}

// currently only supports JSON path.
func getFileURLValues(v *url.Values, revision string, query *Query) (err error) {
	if err = setJSONPaths(v, query); err != nil {
		return
	}

	if len(revision) != 0 {
		// have both of the jsonPath and the revision
		v.Set("revision", revision)
	}

	return
}

func setJSONPaths(v *url.Values, query *Query) error {
	if query.Type == JSONPath {
		if !strings.HasSuffix(strings.ToLower(query.Path), "json") {
			return fmt.Errorf("the extension of the file should be .json (path: %v)", query.Path)
		}

		for _, jsonPath := range query.Expressions {
			v.Add("jsonpath", jsonPath)
		}
	}

	return nil
}
