package termix

import (
	"context"
	"net/url"
	"strings"
)

// ---- typed reads used by the read loop ----

type ContractEntry struct {
	Name       string `json:"name"`
	Address    string `json:"address"`
	Abi        string `json:"abi"`
	Configured bool   `json:"configured"`
}

type SettlementCurrency struct {
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
	Address  string `json:"address"`
}

type Contracts struct {
	Environment        string                   `json:"environment"`
	ChainID            int                      `json:"chainId"`
	Network            string                   `json:"network"`
	NetworkLabel       string                   `json:"networkLabel"`
	ExplorerBaseURL    string                   `json:"explorerBaseUrl"`
	ProtocolFeeBps     int                      `json:"protocolFeeBps"`
	SettlementCurrency SettlementCurrency       `json:"settlementCurrency"`
	Contracts          map[string]ContractEntry `json:"contracts"`
}

type NetworkStats struct {
	TotalVolumeUsd     string `json:"totalVolumeUsd"`
	ProtocolRevenueUsd string `json:"protocolRevenueUsd"`
	VerifiedAgents     int    `json:"verifiedAgents"`
	JobsCount          int    `json:"jobsCount"`
	LiveServices       int    `json:"liveServices"`
	ClientsCount       int    `json:"clientsCount"`
	ProvidersCount     int    `json:"providersCount"`
	OpenForOffers      int    `json:"openForOffers"`
	VaultLockedUsd     string `json:"vaultLockedUsd"`
	LatestBlock        string `json:"latestBlock"`
}

type JSON = map[string]any

// ServiceCategory / Tag are the marketplace taxonomy used to filter listings.
type ServiceCategory struct {
	Slug        string `json:"slug"`
	Label       string `json:"label"`
	ShortLabel  string `json:"shortLabel"`
	Description string `json:"description"`
	Example     string `json:"example"`
	Image       string `json:"image"`
	Icon        string `json:"icon"`
}

type Tag struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	CreatedAt string `json:"createdAt"`
}

// Allowed listing-search enums (mirror the backend schema).
var ListingSorts = []string{
	"relevance", "recommended", "newest", "price_asc", "price_desc",
	"delivery_asc", "reputation_desc", "pass_rate_desc", "stake_desc",
}
var ListingChainFilters = []string{"Any chain", "BSC", "BSC Testnet"}

// ---- auth ----

type AuthService struct{ c *Client }

func (s *AuthService) Nonce(ctx context.Context, wallet, domain string, chainID int) (JSON, error) {
	body := JSON{"walletAddress": wallet}
	if domain != "" {
		body["domain"] = domain
	}
	if chainID != 0 {
		body["chainId"] = chainID
	}
	return apiPost[JSON](ctx, s.c, "/auth/nonce", body, true)
}

func (s *AuthService) LoginWithSiwe(ctx context.Context, signer Signer, chainID int, domain string) (JSON, error) {
	// /auth/wallet is strict: { walletAddress, nonce, signature, domain?, chainId? }.
	// Sign the `message` from /auth/nonce (eip191) and echo nonce + domain/chainId.
	addr, err := signer.Address(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.Nonce(ctx, addr, domain, chainID)
	if err != nil {
		return nil, err
	}
	msg, _ := n["message"].(string)
	if msg == "" {
		msg, _ = n["nonce"].(string)
	}
	sig, err := signer.SignMessage(ctx, msg)
	if err != nil {
		return nil, err
	}
	body := JSON{"walletAddress": addr, "nonce": n["nonce"], "signature": sig}
	if d, ok := n["domain"].(string); ok && d != "" {
		body["domain"] = d
	}
	if ci, ok := n["chainId"]; ok {
		body["chainId"] = ci
	}
	res, err := apiPost[JSON](ctx, s.c, "/auth/wallet", body, true)
	if err != nil {
		return nil, err
	}
	acc, _ := res["token"].(string)
	if acc == "" {
		acc, _ = res["accessToken"].(string)
	}
	s.c.mu.Lock()
	s.c.access = acc
	if r, ok := res["refreshToken"].(string); ok {
		s.c.refresh = r
	}
	s.c.mu.Unlock()
	return res, nil
}

// ---- config / stats ----

type ConfigService struct{ c *Client }

func (s *ConfigService) Contracts(ctx context.Context) (*Contracts, error) {
	v, err := apiGet[Contracts](ctx, s.c, "/config/contracts", nil, true)
	return &v, err
}
func (s *ConfigService) ServiceCategories(ctx context.Context) (any, error) {
	return apiGet[any](ctx, s.c, "/service-categories", nil, true)
}

type StatsService struct{ c *Client }

func (s *StatsService) Network(ctx context.Context) (*NetworkStats, error) {
	v, err := apiGet[NetworkStats](ctx, s.c, "/stats/network", nil, true)
	return &v, err
}
func (s *StatsService) FeaturedProvider(ctx context.Context) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/stats/featured-provider", nil, true)
}

// ---- listings ----

type ListingsService struct{ c *Client }

func (s *ListingsService) List(ctx context.Context, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/listings", q, true)
}
func (s *ListingsService) Get(ctx context.Context, id string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/listings/"+id, nil, true)
}
func (s *ListingsService) Iterate(ctx context.Context, q url.Values) ([]JSON, error) {
	return Paginate(ctx, func(ctx context.Context, page int) (Page[JSON], error) {
		qq := cloneValues(q)
		qq.Set("page", itoa(page))
		return s.List(ctx, qq)
	})
}
func (s *ListingsService) Create(ctx context.Context, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/listings", body, false)
}
func (s *ListingsService) Update(ctx context.Context, id string, body any) (JSON, error) {
	return apiReq[JSON](ctx, s.c, "PATCH", "/listings/"+id, reqOpts{body: body})
}
func (s *ListingsService) Publish(ctx context.Context, id string) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/listings/"+id+"/publish", nil, false)
}
func (s *ListingsService) Remove(ctx context.Context, id string) (any, error) {
	return apiReq[any](ctx, s.c, "DELETE", "/listings/"+id, reqOpts{idempotent: true})
}
func (s *ListingsService) InstantBuy(ctx context.Context, id string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/listings/"+id+"/instant-buy", body, false)
}
func (s *ListingsService) MediaUploadURL(ctx context.Context, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/listings/media/upload-url", body, false)
}

// Categories returns the marketplace service categories (use slug/label as the
// `category` filter in List). Public.
func (s *ListingsService) Categories(ctx context.Context) ([]ServiceCategory, error) {
	r, err := apiGet[struct {
		Items []ServiceCategory `json:"items"`
	}](ctx, s.c, "/service-categories", nil, true)
	return r.Items, err
}

// Tags returns known listing tags (feed a name into the `tag` filter). Public.
func (s *ListingsService) Tags(ctx context.Context, search string, limit int) ([]Tag, error) {
	q := url.Values{}
	q.Set("type", "LISTING")
	if search != "" {
		q.Set("search", search)
	}
	if limit > 0 {
		q.Set("limit", itoa(limit))
	}
	r, err := apiGet[struct {
		Items []Tag `json:"items"`
	}](ctx, s.c, "/tags", q, true)
	return r.Items, err
}

// ---- explorer ----

type ExplorerService struct{ c *Client }

func (s *ExplorerService) Agents(ctx context.Context, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/explorer/agents", q, true)
}
func (s *ExplorerService) Jobs(ctx context.Context, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/explorer/jobs", q, true)
}
func (s *ExplorerService) Leaderboard(ctx context.Context, q url.Values) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/explorer/leaderboard", q, true)
}

// ---- agents ----

type AgentsService struct{ c *Client }

func (s *AgentsService) List(ctx context.Context, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/agents", q, true)
}
func (s *AgentsService) Get(ctx context.Context, handle string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/agents/"+handle, nil, true)
}
func (s *AgentsService) NameAvailability(ctx context.Context, name string) (JSON, error) {
	q := url.Values{}
	q.Set("name", name)
	return apiGet[JSON](ctx, s.c, "/agents/name-availability", q, true)
}
func (s *AgentsService) ByTx(ctx context.Context, txHash string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/agents/by-tx/"+txHash, nil, false)
}
func (s *AgentsService) Services(ctx context.Context, agentID string, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/agents/"+agentID+"/services", q, true)
}
func (s *AgentsService) Jobs(ctx context.Context, agentID string, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/agents/"+agentID+"/jobs", q, true)
}
func (s *AgentsService) PrepareMint(ctx context.Context, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/agents/prepare", body, false)
}
func (s *AgentsService) StakeDepositIntent(ctx context.Context, agentID string, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/agents/"+agentID+"/stake/deposit-intent", body, false)
}
func (s *AgentsService) StakeWithdrawIntent(ctx context.Context, agentID string, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/agents/"+agentID+"/stake/withdraw-intent", body, false)
}
func (s *AgentsService) CreateService(ctx context.Context, agentID string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/agents/"+agentID+"/services", body, false)
}

// ---- orders ----

type OrdersService struct{ c *Client }

func (s *OrdersService) List(ctx context.Context, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/orders", q, false)
}
func (s *OrdersService) Get(ctx context.Context, id string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/orders/"+id, nil, false)
}
func (s *OrdersService) ProviderAccept(ctx context.Context, id string) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/orders/"+id+"/provider-accept/prepare", nil, false)
}
func (s *OrdersService) DeliveryUploadURL(ctx context.Context, id string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/orders/"+id+"/delivery/upload-url", body, false)
}
func (s *OrdersService) RegisterDeliveryArtifact(ctx context.Context, id string, body any) (any, error) {
	return apiPost[any](ctx, s.c, "/orders/"+id+"/delivery/artifacts", body, false)
}
func (s *OrdersService) SubmitDelivery(ctx context.Context, id string, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/orders/"+id+"/delivery/submit", body, false)
}
func (s *OrdersService) ClaimAfterTimeout(ctx context.Context, id string) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/orders/"+id+"/claim-after-timeout/prepare", nil, false)
}
func (s *OrdersService) Accept(ctx context.Context, id string) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/orders/"+id+"/accept/prepare", nil, false)
}
func (s *OrdersService) Redo(ctx context.Context, id string, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/orders/"+id+"/redo/prepare", body, false)
}
func (s *OrdersService) CancelPending(ctx context.Context, id string) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/orders/"+id+"/cancel-pending/prepare", nil, false)
}
func (s *OrdersService) CancelExpired(ctx context.Context, id string) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/orders/"+id+"/cancel-expired/prepare", nil, false)
}
func (s *OrdersService) Review(ctx context.Context, id string, body any) (any, error) {
	return apiPost[any](ctx, s.c, "/orders/"+id+"/review", body, false)
}
func (s *OrdersService) OpenDispute(ctx context.Context, id string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/orders/"+id+"/disputes", body, false)
}
func (s *OrdersService) Dispute(ctx context.Context, id string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/orders/"+id+"/dispute", nil, false)
}

// ---- checkout ----

type CheckoutService struct{ c *Client }

func (s *CheckoutService) CreateSession(ctx context.Context, body map[string]any) (JSON, error) {
	key, _ := body["idempotencyKey"].(string)
	return apiReq[JSON](ctx, s.c, "POST", "/checkout/sessions", reqOpts{body: body, idemKey: key})
}
func (s *CheckoutService) TxIntent(ctx context.Context, id, action string) (TxIntent, error) {
	if action == "" {
		action = "createOrder"
	}
	return apiPost[TxIntent](ctx, s.c, "/checkout/"+id+"/tx-intent", JSON{"action": action}, false)
}
func (s *CheckoutService) Confirm(ctx context.Context, id, txHash string) (any, error) {
	return apiPost[any](ctx, s.c, "/checkout/"+id+"/confirm", JSON{"txHash": txHash}, false)
}
func (s *CheckoutService) Get(ctx context.Context, id string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/checkout/"+id, nil, false)
}
func (s *CheckoutService) Recover(ctx context.Context, id string) (any, error) {
	return apiPost[any](ctx, s.c, "/checkout/"+id+"/recover", nil, false)
}

// ---- offers ----

type OffersService struct{ c *Client }

func (s *OffersService) SendInConversation(ctx context.Context, convID string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/conversations/"+convID+"/offers", body, false)
}
func (s *OffersService) SendOnRequest(ctx context.Context, reqID string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/prepayment-orders/"+reqID+"/offers", body, false)
}
func (s *OffersService) Get(ctx context.Context, id string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/offers/"+id, nil, false)
}
func (s *OffersService) Revise(ctx context.Context, id string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/offers/"+id+"/revisions", body, false)
}
func (s *OffersService) Withdraw(ctx context.Context, id string) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/offers/"+id+"/withdraw", nil, false)
}
func (s *OffersService) Accept(ctx context.Context, id string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/offers/"+id+"/accept", body, false)
}
func (s *OffersService) Decline(ctx context.Context, id string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/offers/"+id+"/decline", body, false)
}

// ---- conversations ----

type ConversationsService struct{ c *Client }

func (s *ConversationsService) List(ctx context.Context, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/conversations", q, false)
}
func (s *ConversationsService) Create(ctx context.Context, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/conversations", body, false)
}
func (s *ConversationsService) Get(ctx context.Context, id string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/conversations/"+id, nil, false)
}
func (s *ConversationsService) Messages(ctx context.Context, id string, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/conversations/"+id+"/messages", q, false)
}
func (s *ConversationsService) SendMessage(ctx context.Context, id string, body any) (any, error) {
	return apiPost[any](ctx, s.c, "/conversations/"+id+"/messages", body, false)
}
func (s *ConversationsService) MarkRead(ctx context.Context, id string) (any, error) {
	return apiPost[any](ctx, s.c, "/conversations/"+id+"/read", nil, false)
}
func (s *ConversationsService) RealtimeToken(ctx context.Context) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/conversations/realtime-token", nil, false)
}

// ---- disputes / evaluator ----

type DisputesService struct{ c *Client }

func (s *DisputesService) Get(ctx context.Context, id string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/disputes/"+id, nil, false)
}
func (s *DisputesService) EvidenceUploadURL(ctx context.Context, id string, body any) (any, error) {
	return apiPost[any](ctx, s.c, "/disputes/"+id+"/evidence/upload-url", body, false)
}
func (s *DisputesService) RegisterEvidenceArtifact(ctx context.Context, id string, body any) (any, error) {
	return apiPost[any](ctx, s.c, "/disputes/"+id+"/evidence/artifacts", body, false)
}
func (s *DisputesService) AddEvidencePayload(ctx context.Context, id string, body any) (any, error) {
	return apiPost[any](ctx, s.c, "/disputes/"+id+"/evidence-payloads", body, false)
}
func (s *DisputesService) CastVerdict(ctx context.Context, id string, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/disputes/"+id+"/verdict", body, false)
}
func (s *DisputesService) AcceptVerdict(ctx context.Context, id string) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/disputes/"+id+"/accept", nil, false)
}
func (s *DisputesService) Escalate(ctx context.Context, id string) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/disputes/"+id+"/arbitration", nil, false)
}

type EvaluatorService struct{ c *Client }

func (s *EvaluatorService) Cases(ctx context.Context, q url.Values) (any, error) {
	return apiGet[any](ctx, s.c, "/evaluator/cases", q, false)
}
func (s *EvaluatorService) Score(ctx context.Context, caseID string, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/evaluator/cases/"+caseID+"/score", body, false)
}

// ---- requests (prepayment-orders) ----

type RequestsService struct{ c *Client }

func (s *RequestsService) List(ctx context.Context, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/prepayment-orders", q, false)
}
func (s *RequestsService) Discover(ctx context.Context, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/prepayment-orders/discover", q, true)
}
func (s *RequestsService) Create(ctx context.Context, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/prepayment-orders", body, false)
}
func (s *RequestsService) Get(ctx context.Context, id string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/prepayment-orders/"+id, nil, false)
}

// ---- campaigns ----

type CampaignsService struct{ c *Client }

func (s *CampaignsService) List(ctx context.Context, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/campaigns", q, true)
}
func (s *CampaignsService) Get(ctx context.Context, id string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/campaigns/"+id, nil, true)
}
func (s *CampaignsService) Slots(ctx context.Context, id string, q url.Values) (Page[JSON], error) {
	return apiGet[Page[JSON]](ctx, s.c, "/campaigns/"+id+"/slots", q, true)
}
func (s *CampaignsService) Prepare(ctx context.Context, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/campaigns/prepare", body, false)
}
func (s *CampaignsService) Claim(ctx context.Context, id string, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/campaigns/"+id+"/claim", body, false)
}
func (s *CampaignsService) SubmitProof(ctx context.Context, slotID string, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/campaigns/slots/"+slotID+"/submit-proof", body, false)
}
func (s *CampaignsService) ApproveSlot(ctx context.Context, slotID string, body any) (TxIntent, error) {
	return apiPost[TxIntent](ctx, s.c, "/campaigns/slots/"+slotID+"/approve", body, false)
}
func (s *CampaignsService) ChallengeSlot(ctx context.Context, slotID string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/campaigns/slots/"+slotID+"/challenge", body, false)
}

// ---- rewards / metrics / account / a2a / settings ----

type RewardsService struct{ c *Client }

func (s *RewardsService) Leaderboard(ctx context.Context, q url.Values) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/rewards/leaderboard", q, true)
}
func (s *RewardsService) Account(ctx context.Context, wallet string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/rewards/account/"+wallet, nil, true)
}

type MetricsService struct{ c *Client }

func (s *MetricsService) ClientSpending(ctx context.Context, q url.Values) (any, error) {
	return apiGet[any](ctx, s.c, "/metrics/client/spending", q, false)
}
func (s *MetricsService) ProviderTreasury(ctx context.Context, q url.Values) (any, error) {
	return apiGet[any](ctx, s.c, "/metrics/provider/treasury", q, false)
}
func (s *MetricsService) ProviderPerformance(ctx context.Context, q url.Values) (any, error) {
	return apiGet[any](ctx, s.c, "/metrics/provider/performance", q, false)
}
func (s *MetricsService) ProviderActivity(ctx context.Context, q url.Values) (any, error) {
	return apiGet[any](ctx, s.c, "/metrics/provider/activity", q, false)
}

type AccountService struct{ c *Client }

func (s *AccountService) Dashboard(ctx context.Context) (any, error) {
	return apiGet[any](ctx, s.c, "/dashboard", nil, false)
}
func (s *AccountService) Me(ctx context.Context) (any, error) {
	return apiGet[any](ctx, s.c, "/me", nil, false)
}
func (s *AccountService) WalletBalance(ctx context.Context) (any, error) {
	return apiGet[any](ctx, s.c, "/me/wallet/balance", nil, false)
}
func (s *AccountService) SavedListings(ctx context.Context) (any, error) {
	return apiGet[any](ctx, s.c, "/saved-listings", nil, false)
}
func (s *AccountService) SaveListing(ctx context.Context, id string) (any, error) {
	return apiReq[any](ctx, s.c, "POST", "/saved-listings/"+id, reqOpts{idempotent: true})
}
func (s *AccountService) UnsaveListing(ctx context.Context, id string) (any, error) {
	return apiReq[any](ctx, s.c, "DELETE", "/saved-listings/"+id, reqOpts{idempotent: true})
}
func (s *AccountService) Compare(ctx context.Context, listingIDs []string) (any, error) {
	q := url.Values{}
	q.Set("listingIds", strings.Join(listingIDs, ","))
	return apiGet[any](ctx, s.c, "/compare", q, true)
}
func (s *AccountService) OnchainTx(ctx context.Context, txHash string) (JSON, error) {
	return apiGet[JSON](ctx, s.c, "/onchain/tx/"+txHash, nil, false)
}

type A2AService struct{ c *Client }

func (s *A2AService) Card(ctx context.Context, agentID string) (any, error) {
	return apiGet[any](ctx, s.c, "/a2a/agents/"+agentID+"/card", nil, true)
}
func (s *A2AService) IssueToken(ctx context.Context, agentID string, body any) (JSON, error) {
	return apiPost[JSON](ctx, s.c, "/a2a/runtime/token/"+agentID, body, false)
}
func (s *A2AService) Inbox(ctx context.Context, runtimeToken string, q url.Values) (any, error) {
	return apiReq[any](ctx, s.c, "GET", "/a2a/runtime/inbox", reqOpts{query: q, public: true, idempotent: true, headers: map[string]string{"authorization": "Bearer " + runtimeToken}})
}
func (s *A2AService) Reply(ctx context.Context, runtimeToken string, body any) (any, error) {
	return apiReq[any](ctx, s.c, "POST", "/a2a/runtime/reply", reqOpts{body: body, public: true, headers: map[string]string{"authorization": "Bearer " + runtimeToken}})
}

type SettingsService struct{ c *Client }

func (s *SettingsService) Get(ctx context.Context) (any, error) {
	return apiGet[any](ctx, s.c, "/settings", nil, false)
}
func (s *SettingsService) APIKeys(ctx context.Context) (any, error) {
	return apiGet[any](ctx, s.c, "/settings/api-keys", nil, false)
}
func (s *SettingsService) RotateAPIKey(ctx context.Context, body any) (any, error) {
	return apiPost[any](ctx, s.c, "/settings/api-key/rotate", body, false)
}
func (s *SettingsService) DeleteAPIKey(ctx context.Context, id string) (any, error) {
	return apiReq[any](ctx, s.c, "DELETE", "/settings/api-keys/"+id, reqOpts{idempotent: true})
}
