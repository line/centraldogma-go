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

func setURLValue(v *url.Values, key, value string) {
	if len(value) > 0 {
		v.Set(key, value)
	}
}

func setFromTo(v *url.Values, from, to string) {
	setURLValue(v, "from", from)
	setURLValue(v, "to", to)
}

func setRevision(v *url.Values, revision string) {
	setURLValue(v, "revision", revision)
}

func setPath(v *url.Values, path string) {
	setURLValue(v, "path", path)
}

func setPathPattern(v *url.Values, pathPattern string) {
	setURLValue(v, "pathPattern", pathPattern)
}

func setMaxCommits(v *url.Values, maxCommits int) {
	if maxCommits != 0 {
		setURLValue(v, "maxCommits", strconv.Itoa(maxCommits))
	}
}

// currently only supports JSON path.
func getFileURLValues(v *url.Values, revision string, query *Query) (err error) {
	if err = setJSONPaths(v, query); err == nil {
		// have both of the jsonPath and the revision
		setRevision(v, revision)
	}
	return
}

func setJSONPaths(v *url.Values, query *Query) (err error) {
	if query.Type == JSONPath {
		if !strings.HasSuffix(strings.ToLower(query.Path), "json") {
			err = fmt.Errorf("the extension of the file should be .json (path: %v)", query.Path)
		} else {
			for _, jsonPath := range query.Expressions {
				v.Add("jsonpath", jsonPath)
			}
		}
	}
	return
}
