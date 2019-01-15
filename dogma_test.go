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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/veqryn/h2c"
	"golang.org/x/net/http2"
	"golang.org/x/oauth2"
)

const token = "appToken-fdbe78b0-1d0c-4978-bbb1-9bc7106dad36"

// setup sets up a test HTTP/2 server along with a centraldogma.Client that is
// configured to talk to that test server. Tests should register handlers on
// mux which provide mock responses for the API method being tested.
func setup() (*Client, *http.ServeMux, func()) {
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{
		CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		NextProtos:   []string{http2.NextProtoTLS},
	}
	server.StartTLS()
	certPool := x509.NewCertPool()
	certPool.AddCert(server.Certificate())
	tlsClientConfig := &tls.Config{RootCAs: certPool}

	c, _ := NewClientWithToken(normalizedURL(server.URL).String(), token, &http2.Transport{TLSClientConfig: tlsClientConfig})
	mc, _ := GlobalPrometheusMetricCollector(DefaultMetricCollectorConfig("testClient"))
	c.SetMetricCollector(mc)
	return c, mux, server.Close
}

func normalizedURL(url string) *url.URL {
	normalizedUrl, _ := normalizeURL(url)
	return normalizedUrl
}

func setupH2C() (*Client, *http.ServeMux, func()) {
	mux := http.NewServeMux()
	h2cWrapper := &h2c.HandlerH2C{
		Handler:  mux,
		H2Server: &http2.Server{},
	}
	server := httptest.NewServer(h2cWrapper)
	c, _ := NewClientWithToken(server.URL, token, nil)
	return c, mux, server.Close
}

func setupH1C() (c *Client, mux *http.ServeMux, teardown func()) {
	mux = http.NewServeMux()
	server := httptest.NewServer(mux)
	c, _ = NewClientWithToken(normalizedURL(server.URL).String(), token, http.DefaultTransport)
	return c, mux, server.Close
}

func testMethod(t *testing.T, req *http.Request, want string) {
	if got := req.Method; got != want {
		t.Errorf("Request method: %v, want %v", got, want)
	}
}

func testHeader(t *testing.T, req *http.Request, header string, want string) {
	if got := req.Header.Get(header); got != want {
		t.Errorf("Header.Get(%q) returned %q, want %q", header, got, want)
	}
}

func testAuthorization(t *testing.T, req *http.Request) {
	want := "Bearer " + token
	if got := req.Header.Get("authorization"); got != want {
		t.Errorf("authorization: %q, want %q", got, want)
	}
}

func testURLQuery(t *testing.T, r *http.Request, key, want string) {
	if got := r.URL.Query().Get(key); got != want {
		t.Errorf("Query.Get(%q) returned %q, want %q", key, got, want)
	}
}

func testBody(t *testing.T, req *http.Request, want string) {
	b, err := ioutil.ReadAll(req.Body)
	if err != nil {
		t.Errorf("Error reading request body: %v", err)
	}
	if got := string(b); got != want {
		t.Errorf("Request body: %v, want %v", got, want)
	}
}

func testStatusCode(t *testing.T, httpStatusCode int, want int) {
	if got := httpStatusCode; got != want {
		t.Errorf("Response status: %v, want %v", got, want)
	}
}

func testString(t *testing.T, got, want, name string) {
	if got != want {
		t.Errorf("%v: %v, want %v", name, got, want)
	}
}

func TestNormalizeURL(t *testing.T) {
	var tests = []struct {
		baseURL string
		want    string
	}{
		{"", defaultBaseURL},
		{"central-dogma.com", "https://central-dogma.com/"},
		{"central-dogma.com:443", "https://central-dogma.com:443/"},
		{"http://central-dogma.com", "http://central-dogma.com/"},
		{"http://central-dogma.com:80", "http://central-dogma.com:80/"},
	}

	for _, test := range tests {
		var got *url.URL
		if got, _ = normalizeURL(test.baseURL); got.String() != test.want {
			t.Errorf("newClientWithHTTPClient BaseURL is %v, want %v", got, test.want)
		}
	}
}

func TestDefaultHTTP2Transport(t *testing.T) {
	normalizedH2C := "http://localhost/"
	normalizedH2 := "https://localhost/"

	h2cTransport, _ := DefaultHTTP2Transport(normalizedH2C)
	if h2cTransport.AllowHTTP != true {
		t.Errorf("h2cTransport.AllowHTTP is %t, want %t", h2cTransport.AllowHTTP, true)
	}
	if h2cTransport.DialTLS == nil {
		t.Errorf("h2cTransport.DialTLS is nil")
	}

	h2Transport, _ := DefaultHTTP2Transport(normalizedH2)
	if h2Transport.AllowHTTP != false {
		t.Errorf("h2Transport.AllowHTTP is %t, want %t", h2cTransport.AllowHTTP, false)
	}
	if h2Transport.DialTLS != nil { // h2Transport.DialTLS should be nil so that dialTLSDefault() is called.
		t.Errorf("h2Transport.DialTLS is not nil")
	}
}

func TestDefaultOauth2Transport(t *testing.T) {
	baseURL := "https://localhost/"
	http2Transport, _ := DefaultHTTP2Transport(baseURL)
	oauth2Transport, _ := DefaultOauth2Transport(baseURL, "myToken", http2Transport)
	if oauth2Transport.Base != http2Transport {
		t.Errorf("oauth2Transport.Base is %+v, want %+v", oauth2Transport.Base, http2Transport)
	}

	token, _ := oauth2Transport.Source.Token()
	if token.AccessToken != "myToken" {
		t.Errorf("oauth2Transport.Source.Token() returned %s, want %s", token.AccessToken, "myToken")
	}

	// Error when DefaultOauth2Transport is called with oauth2.Transport
	transport, err := DefaultOauth2Transport(baseURL, "myToken", oauth2Transport)
	if err == nil {
		t.Errorf("DefaultOauth2Transport returned %+v, want nil", transport)
	}
}

type helloArmeria struct {
	Hello string `json:"hello"`
}

func TestNewClientWithToken_h2(t *testing.T) {
	client, mux, teardown := setup()
	defer teardown()

	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		testString(t, r.Proto, "HTTP/2.0", "protocol")
		testString(t, r.TLS.NegotiatedProtocol, "h2", "NegotiatedProtocol")
		testAuthorization(t, r)
		fmt.Fprint(w, `{"hello":"Armeria"}`)
	})
	req, _ := http.NewRequest(http.MethodGet, client.baseURL.String()+"test", nil)
	res, _ := client.client.Do(req)
	testString(t, res.Proto, "HTTP/2.0", "protocol")
	testString(t, res.TLS.NegotiatedProtocol, "h2", "NegotiatedProtocol")
	hello := &helloArmeria{}

	defer res.Body.Close()
	json.NewDecoder(res.Body).Decode(hello)
	testString(t, hello.Hello, "Armeria", "hello")
}

func TestNewClientWithToken_h2c(t *testing.T) {
	client, mux, teardown := setupH2C()
	defer teardown()

	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		testString(t, r.Proto, "HTTP/2.0", "protocol")
		if r.TLS != nil {
			t.Errorf("r.TLS is not nil: %+v", r.TLS)
		}
		testAuthorization(t, r)
		fmt.Fprint(w, `{"hello":"Armeria"}`)
	})

	req, _ := http.NewRequest(http.MethodGet, client.baseURL.String()+"test", nil)
	res, _ := client.client.Do(req)
	testString(t, res.Proto, "HTTP/2.0", "protocol")
	if res.TLS != nil {
		t.Errorf("r.TLS is not nil: %+v", res.TLS)
	}
	hello := &helloArmeria{}

	defer res.Body.Close()
	json.NewDecoder(res.Body).Decode(hello)
	testString(t, hello.Hello, "Armeria", "hello")
}

func TestNewClientWithToken_h1c(t *testing.T) {
	client, mux, teardown := setupH1C()
	defer teardown()

	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		testString(t, r.Proto, "HTTP/1.1", "protocol")
		if r.TLS != nil {
			t.Errorf("r.TLS is not nil: %+v", r.TLS)
		}
		testAuthorization(t, r)
		fmt.Fprint(w, `{"hello":"Armeria"}`)
	})

	req, _ := http.NewRequest(http.MethodGet, client.baseURL.String()+"test", nil)
	res, _ := client.client.Do(req)
	testString(t, res.Proto, "HTTP/1.1", "protocol")
	if res.TLS != nil {
		t.Errorf("r.TLS is not nil: %+v", res.TLS)
	}
	hello := &helloArmeria{}

	defer res.Body.Close()
	json.NewDecoder(res.Body).Decode(hello)
	testString(t, hello.Hello, "Armeria", "hello")
}

func TestNewClientWithHTTPClient(t *testing.T) {
	myClient := &http.Client{}
	client, _ := newClientWithHTTPClient(normalizedURL(defaultBaseURL), myClient)
	if client.baseURL.String() != defaultBaseURL {
		t.Errorf("newClientWithHTTPClient BaseURL is %v, want %v", client.baseURL, defaultBaseURL)
	}

	if client.client != myClient {
		t.Errorf("newClientWithHTTPClient client is %v, want %v", client.client, myClient)
	}
}

func TestNewClientWithHTTPClient1(t *testing.T) {
	normalizedURL := "https://localhost/"

	httpTransport := &http.Transport{}
	// The httpTransport is wrapped with oauth2.Transport.
	client, _ := newOauth2HTTP2Client(normalizedURL, "myToken", httpTransport)
	oauth2Transport, _ := client.Transport.(*oauth2.Transport)
	if oauth2Transport.Base != httpTransport {
		t.Errorf("newOauth2HTTP2Client transport is %+v, want http.Transport", oauth2Transport.Base)
	}

	// If the transport is nil http2.Transport is used.
	client, _ = newOauth2HTTP2Client(normalizedURL, "myToken", nil)
	oauth2Transport, _ = client.Transport.(*oauth2.Transport)
	_, ok := oauth2Transport.Base.(*http2.Transport)
	if !ok {
		t.Errorf("newOauth2HTTP2Client transport is %+v, want http2.Transport", client.Transport)
	}

	// If an oauth2Transport is used, it doesn't wrap and just return it.
	client, _ = newOauth2HTTP2Client(normalizedURL, "myToken", oauth2Transport)
	newOauth2Transport, _ := client.Transport.(*oauth2.Transport)
	if oauth2Transport != newOauth2Transport {
		t.Errorf("newOauth2HTTP2Client transport is %+v, want %+v", client.Transport, oauth2Transport)
	}
}
