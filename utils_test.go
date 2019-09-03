package centraldogma

import (
	"math/rand"
	"net/url"
	"testing"
	"time"
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

func TestNextDelay(t *testing.T) {
	rand.Seed(0)

	delay := nextDelay(0)
	testDelay(t, delay, "1.073713114s")
	delay = nextDelay(1)
	testDelay(t, delay, "1.920167497s")
	delay = nextDelay(2)
	testDelay(t, delay, "4.320190114s")
	delay = nextDelay(3)
	testDelay(t, delay, "6.966156812s")
	delay = nextDelay(4)
	testDelay(t, delay, "16.842968555s")
}

func testDelay(t *testing.T, delay time.Duration, want string) {
	if delay.String() != want {
		t.Errorf("delay: %v, want %v", delay, want)
	}
}
