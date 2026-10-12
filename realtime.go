package termix

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Event is one Server-Sent Event from the realtime stream.
type Event struct {
	Event string
	ID    string
	Data  string
}

// Subscribe opens the Centrifugo SSE stream (no webhooks) and invokes onEvent per
// event. Returns a stop function; call it to cancel. Requires a session token.
func (c *Client) Subscribe(ctx context.Context, onEvent func(Event)) (func(), error) {
	tok := c.token()
	if tok == "" {
		return nil, fmt.Errorf("realtime subscribe requires an authenticated session (no token)")
	}
	endpoint, err := url.Parse(c.baseURL + "/realtime/sse")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("token", tok)
	endpoint.RawQuery = query.Encode()

	ctx2, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(ctx2, "GET", endpoint.String(), nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("accept", "text/event-stream")
	streamClient := *c.http
	streamClient.Timeout = 0 // keep the configured transport, but do not time out a live stream
	resp, err := streamClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("realtime SSE failed: HTTP %d", resp.StatusCode)
	}
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		var ev, id string
		var data []string
		flush := func() {
			if len(data) > 0 || ev != "" {
				onEvent(Event{Event: ev, ID: id, Data: strings.Join(data, "\n")})
			}
			ev, id, data = "", "", nil
		}
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				flush()
				continue
			}
			if strings.HasPrefix(line, ":") {
				continue
			}
			k, v, _ := strings.Cut(line, ":")
			v = strings.TrimPrefix(v, " ")
			switch k {
			case "event":
				ev = v
			case "id":
				id = v
			case "data":
				data = append(data, v)
			}
		}
	}()
	return cancel, nil
}
