package centraldogma

import (
	"net/url"
	"testing"
)

func TestSetMalformJSONPath(t *testing.T) {
	v := &url.Values{}
	query := Query{
		Path: "test.yaml",
		Type: JSONPath,
	}
	if setJSONPaths(v, &query) == nil {
		t.Fatal()
	}
}
