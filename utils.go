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
	"math"
	"math/rand"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	delayOnSuccess = 1 * time.Second
	minInterval    = delayOnSuccess * 2
	maxInterval    = 1 * time.Minute
	jitterRate     = 0.2
	maxInt63       = int64(^uint64(0) >> 1)
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

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

func nextDelay(numAttemptsSoFar int) time.Duration {
	var nextDelay time.Duration
	if numAttemptsSoFar == 1 {
		nextDelay = minInterval
	} else {
		calculatedDelay := saturatedMultiply(minInterval, math.Pow(2.0, float64(numAttemptsSoFar-1)))
		if calculatedDelay > maxInterval {
			nextDelay = maxInterval
		} else {
			nextDelay = calculatedDelay
		}
	}
	minJitter := int64(float64(nextDelay) * (1 - jitterRate))
	maxJitter := int64(float64(nextDelay) * (1 + jitterRate))
	bound := maxJitter - minJitter + 1
	random := random(bound)
	result := saturatedAdd(minJitter, random)
	if result < 0 {
		result = 0
	}
	return time.Duration(result)
}

func saturatedMultiply(left time.Duration, right float64) time.Duration {
	result := float64(left) * right
	if result > float64(maxInt63) {
		return time.Duration(maxInt63)
	}
	return time.Duration(result)
}

func random(bound int64) int64 {
	mask := bound - 1
	result := rand.Int63()

	if bound&mask == 0 {
		// power of two
		result &= mask
	} else { // reject over-represented candidates
		for u := result >> 1; u+mask-result < 0; u = rand.Int63() >> 1 {
			result = u % bound
		}
	}
	return result
}

// This code is from Guava library.
func saturatedAdd(a, b int64) int64 {
	naiveSum := a + b
	if a^b < 0 || a^naiveSum >= 0 {
		// If a and b have different signs or a has the same sign as the result then there was no overflow.
		return naiveSum
	}
	return maxInt63 + ((naiveSum >> (64 - 1)) ^ 1)
}
