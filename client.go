package termix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Options configure a Client.
type Options struct {
	Chain        string // bsc (default) | base | rh; ignored when BaseURL set
	BaseURL      string // explicit override (self-host / dev / testnet)
	Timeout      time.Duration
	MaxRetries   int
	Token        string // session JWT
	RefreshToken string
	APIKey       string // programmatic auth (used when no session token)
	Signer       Signer // inject for mode B (internal sign + broadcast)
	HTTPClient   *http.Client
	Interceptors []Interceptor      // request/response observability hooks
	OnWarning    func(msg string)   // config-drift warnings (default: stderr)
}

// RequestLog / ResponseLog are passed to interceptors (headers are redacted).
type RequestLog struct {
	Method  string
	URL     string
	Headers map[string]string
}
type ResponseLog struct {
	Method     string
	URL        string
	Status     int
	DurationMs int64
}
type Interceptor struct {
	OnRequest  func(RequestLog)
	OnResponse func(ResponseLog)
}

type Client struct {
	baseURL    string
	http       *http.Client
	maxRetries int
	apiKey     string
	Signer     Signer

	mu      sync.Mutex
	access  string
	refresh string

	chain           string
	baseURLOverride bool
	onWarning       func(string)
	interceptors    []Interceptor
	compatChecked   bool

	contracts *Contracts

	Auth          *AuthService
	Config        *ConfigService
	Stats         *StatsService
	Listings      *ListingsService
	Explorer      *ExplorerService
	Agents        *AgentsService
	Orders        *OrdersService
	Offers        *OffersService
	Checkout      *CheckoutService
	Conversations *ConversationsService
	Disputes      *DisputesService
	Evaluator     *EvaluatorService
	Requests      *RequestsService
	Campaigns     *CampaignsService
	Rewards       *RewardsService
	Metrics       *MetricsService
	Account       *AccountService
	A2A           *A2AService
	Settings      *SettingsService
	Flows         *Flows
}

var apiVerRe = regexp.MustCompile(`/api/v\d+$`)

func resolveBaseURL(chain, baseURL string) string {
	if baseURL != "" {
		b := strings.TrimRight(baseURL, "/")
		if apiVerRe.MatchString(b) {
			return b
		}
		return b + "/api/v1"
	}
	if chain == "" {
		chain = DefaultChain
	}
	return Chains[chain].Host + "/api/v1"
}

// New constructs a Client. Call Contracts() once to warm the chain config.
func New(opts Options) *Client {
	hc := opts.HTTPClient
	if hc == nil {
		to := opts.Timeout
		if to == 0 {
			to = 30 * time.Second
		}
		hc = &http.Client{Timeout: to}
	}
	mr := opts.MaxRetries
	if mr == 0 {
		mr = 2
	}
	onWarning := opts.OnWarning
	if onWarning == nil {
		onWarning = func(m string) { fmt.Fprintln(os.Stderr, "[termix] "+m) }
	}
	c := &Client{
		baseURL:         resolveBaseURL(opts.Chain, opts.BaseURL),
		http:            hc,
		maxRetries:      mr,
		apiKey:          opts.APIKey,
		Signer:          opts.Signer,
		access:          opts.Token,
		refresh:         opts.RefreshToken,
		chain:           opts.Chain,
		baseURLOverride: opts.BaseURL != "",
		onWarning:       onWarning,
		interceptors:    opts.Interceptors,
	}
	c.Auth = &AuthService{c}
	c.Config = &ConfigService{c}
	c.Stats = &StatsService{c}
	c.Listings = &ListingsService{c}
	c.Explorer = &ExplorerService{c}
	c.Agents = &AgentsService{c}
	c.Orders = &OrdersService{c}
	c.Offers = &OffersService{c}
	c.Checkout = &CheckoutService{c}
	c.Conversations = &ConversationsService{c}
	c.Disputes = &DisputesService{c}
	c.Evaluator = &EvaluatorService{c}
	c.Requests = &RequestsService{c}
	c.Campaigns = &CampaignsService{c}
	c.Rewards = &RewardsService{c}
	c.Metrics = &MetricsService{c}
	c.Account = &AccountService{c}
	c.A2A = &A2AService{c}
	c.Settings = &SettingsService{c}
	c.Flows = &Flows{c}
	return c
}

// BaseURL exposes the resolved /api/v1 base.
func (c *Client) BaseURL() string { return c.baseURL }

func (c *Client) token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.access != "" {
		return c.access
	}
	return c.apiKey
}

type reqOpts struct {
	query      url.Values
	body       any
	headers    map[string]string
	idempotent bool
	public     bool
	idemKey    string
}

func backoff(attempt int) {
	d := time.Duration(math.Min(1000*math.Pow(2, float64(attempt-1)), 8000)) * time.Millisecond
	time.Sleep(d + time.Duration(rand.Intn(250))*time.Millisecond)
}

func (c *Client) doHTTP(ctx context.Context, method, path string, o reqOpts) ([]byte, int, error) {
	maxRetries := 0
	if o.idempotent {
		maxRetries = c.maxRetries
	}
	var bodyBytes []byte
	if o.body != nil {
		bodyBytes, _ = json.Marshal(o.body)
	}
	u := c.baseURL + path
	if len(o.query) > 0 {
		u += "?" + o.query.Encode()
	}
	attempt, refreshed := 0, false
	for {
		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, reader)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("accept", "application/json")
		if o.body != nil {
			req.Header.Set("content-type", "application/json")
		}
		if o.idemKey != "" {
			req.Header.Set("idempotency-key", o.idemKey)
		}
		for k, v := range o.headers {
			req.Header.Set(k, v)
		}
		if !o.public {
			if tok := c.token(); tok != "" {
				req.Header.Set("authorization", "Bearer "+tok)
			}
		}
		for _, itc := range c.interceptors {
			if itc.OnRequest != nil {
				itc.OnRequest(RequestLog{Method: method, URL: u, Headers: redactHeaders(req.Header)})
			}
		}
		started := time.Now()
		resp, err := c.http.Do(req)
		if err != nil {
			if attempt < maxRetries {
				attempt++
				backoff(attempt)
				continue
			}
			return nil, 0, &NetworkError{Msg: "network error: " + method + " " + path, Err: err}
		}
		for _, itc := range c.interceptors {
			if itc.OnResponse != nil {
				itc.OnResponse(ResponseLog{Method: method, URL: u, Status: resp.StatusCode, DurationMs: time.Since(started).Milliseconds()})
			}
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 401 && !o.public && !refreshed {
			if c.tryRefresh(ctx) {
				refreshed = true
				continue
			}
		}
		if resp.StatusCode >= 400 {
			if (resp.StatusCode >= 500 || resp.StatusCode == 429) && attempt < maxRetries {
				attempt++
				backoff(attempt)
				continue
			}
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			return nil, resp.StatusCode, mapAPIError(resp.StatusCode, m)
		}
		return data, resp.StatusCode, nil
	}
}

func (c *Client) tryRefresh(ctx context.Context) bool {
	c.mu.Lock()
	rt := c.refresh
	c.mu.Unlock()
	if rt == "" {
		return false
	}
	res, err := apiReq[map[string]any](ctx, c, "POST", "/auth/refresh", reqOpts{body: map[string]any{"refreshToken": rt}, public: true})
	if err != nil {
		return false
	}
	acc, _ := res["token"].(string)
	if acc == "" {
		acc, _ = res["accessToken"].(string)
	}
	if acc == "" {
		return false
	}
	c.mu.Lock()
	c.access = acc
	if r, ok := res["refreshToken"].(string); ok {
		c.refresh = r
	}
	c.mu.Unlock()
	return true
}

func apiReq[T any](ctx context.Context, c *Client, method, path string, o reqOpts) (T, error) {
	var out T
	data, _, err := c.doHTTP(ctx, method, path, o)
	if err != nil {
		return out, err
	}
	if len(data) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func apiGet[T any](ctx context.Context, c *Client, path string, q url.Values, public bool) (T, error) {
	return apiReq[T](ctx, c, "GET", path, reqOpts{query: q, public: public, idempotent: true})
}

func apiPost[T any](ctx context.Context, c *Client, path string, body any, public bool) (T, error) {
	return apiReq[T](ctx, c, "POST", path, reqOpts{body: body, public: public})
}

// Contracts returns (and caches) /config/contracts for this deployment.
func (c *Client) Contracts(ctx context.Context) (*Contracts, error) {
	if c.contracts != nil {
		return c.contracts, nil
	}
	cfg, err := c.Config.Contracts(ctx)
	if err != nil {
		return nil, err
	}
	c.contracts = cfg
	c.checkCompat(cfg)
	return cfg, nil
}

// checkCompat warns once if the live backend chainId mismatches the requested named
// chain (catches pointing a client at the wrong backend). Skipped for baseURL overrides.
func (c *Client) checkCompat(cfg *Contracts) {
	if c.compatChecked {
		return
	}
	c.compatChecked = true
	if c.chain != "" && !c.baseURLOverride {
		if expected := Chains[c.chain].ChainID; cfg.ChainID != expected && c.onWarning != nil {
			c.onWarning(fmt.Sprintf("backend chainId %d does not match expected %d for chain %q", cfg.ChainID, expected, c.chain))
		}
	}
}

func redactHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for k := range h {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "idempotency-key" {
			out[k] = "<redacted>"
		} else {
			out[k] = h.Get(k)
		}
	}
	return out
}
