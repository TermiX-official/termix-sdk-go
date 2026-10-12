package termix

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestSubscribeUsesConfiguredHTTPClientWithoutRequestTimeout(t *testing.T) {
	const token = "session+token&scope=realtime"
	events := make(chan Event, 1)
	transportCalled := false
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		transportCalled = true
		if _, hasDeadline := req.Context().Deadline(); hasDeadline {
			t.Fatal("stream request inherited the HTTP client's timeout")
		}
		if got := req.URL.Query().Get("token"); got != token {
			t.Fatalf("decoded token = %q, want %q", got, token)
		}
		if got := req.URL.RawQuery; got != "token=session%2Btoken%26scope%3Drealtime" {
			t.Fatalf("raw query = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("event: update\nid: 7\ndata: ready\n\n")),
			Request:    req,
		}, nil
	})
	client := New(Options{
		BaseURL: "https://example.test",
		Token:   token,
		HTTPClient: &http.Client{
			Transport: transport,
			Timeout:   time.Millisecond,
		},
	})

	stop, err := client.Subscribe(context.Background(), func(event Event) {
		events <- event
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	select {
	case event := <-events:
		if event != (Event{Event: "update", ID: "7", Data: "ready"}) {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime event")
	}
	if !transportCalled {
		t.Fatal("configured transport was not used")
	}
}
