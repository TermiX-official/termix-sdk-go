package termix

import (
	"context"
	"fmt"
	"time"
)

// WaitOpts tunes the state-reconciling pollers used by flows.
type WaitOpts struct {
	Timeout  time.Duration
	Interval time.Duration
}

// FlowResult is returned by every write flow. In mode A (no autoSign) only Intents
// are populated; the caller signs and broadcasts them.
type FlowResult struct {
	Intents  []TxIntent
	TxHashes []string
	Result   JSON
}

type Flows struct{ c *Client }

func (f *Flows) broadcast(ctx context.Context, intent TxIntent, autoSign bool) (string, error) {
	if !autoSign {
		return "", nil
	}
	if f.c.Signer == nil {
		return "", &ChainError{Msg: "autoSign requires a signer on the client (mode B)"}
	}
	// Some intents (e.g. agent mint /agents/prepare) omit chainId; fill from config.
	if intent.ChainID == 0 {
		if cfg, err := f.c.Contracts(ctx); err == nil {
			intent.ChainID = cfg.ChainID
		}
	}
	return broadcastIntent(ctx, f.c.Signer, intent)
}

func (f *Flows) getOrder(ctx context.Context, id string) (JSON, error) { return f.c.Orders.Get(ctx, id) }

func (f *Flows) single(ctx context.Context, orderID string, intent TxIntent, done func(JSON) bool, autoSign bool, wait WaitOpts) (FlowResult, error) {
	h, err := f.broadcast(ctx, intent, autoSign)
	if err != nil {
		return FlowResult{}, err
	}
	if h == "" {
		return FlowResult{Intents: []TxIntent{intent}}, nil
	}
	order, err := WaitFor(ctx, f.orderReader(orderID), done, wait.Timeout, wait.Interval)
	if err != nil {
		return FlowResult{}, err
	}
	return FlowResult{Intents: []TxIntent{intent}, TxHashes: []string{h}, Result: order}, nil
}

func (f *Flows) orderReader(id string) func(context.Context) (JSON, error) {
	return func(ctx context.Context) (JSON, error) { return f.getOrder(ctx, id) }
}

// RegisterAgent mints an agent: name check -> prepare -> (broadcast) -> resolve.
func (f *Flows) RegisterAgent(ctx context.Context, handle string, mint JSON, autoSign bool, wait WaitOpts) (FlowResult, error) {
	avail, err := f.c.Agents.NameAvailability(ctx, handle)
	if err != nil {
		return FlowResult{}, err
	}
	if a, ok := avail["available"].(bool); ok && !a {
		return FlowResult{}, fmt.Errorf("agent handle not available: %s", handle)
	}
	body := JSON{"name": handle}
	for k, v := range mint {
		body[k] = v
	}
	intent, err := f.c.Agents.PrepareMint(ctx, body)
	if err != nil {
		return FlowResult{}, err
	}
	h, err := f.broadcast(ctx, intent, autoSign)
	if err != nil {
		return FlowResult{}, err
	}
	if h == "" {
		return FlowResult{Intents: []TxIntent{intent}}, nil
	}
	read := func(ctx context.Context) (JSON, error) {
		a, e := f.c.Agents.ByTx(ctx, h)
		if IsNotFound(e) {
			return nil, nil
		}
		return a, e
	}
	// The mint tx is broadcast; agent resolution depends on the indexer, which can lag.
	// Never discard a successful broadcast — return the hash even if resolution times out.
	agent, err := WaitFor(ctx, read, func(a JSON) bool { _, ok := a["id"]; return a != nil && ok }, wait.Timeout, wait.Interval)
	if err != nil {
		agent = nil
	}
	return FlowResult{Intents: []TxIntent{intent}, TxHashes: []string{h}, Result: agent}, nil
}

// CompleteCheckout drives a session to a funded order (approve -> wait -> createOrder).
func (f *Flows) CompleteCheckout(ctx context.Context, checkoutID string, autoSign bool, wait WaitOpts) (FlowResult, error) {
	approve, err := f.c.Checkout.TxIntent(ctx, checkoutID, "approveEscrow")
	if err != nil {
		return FlowResult{}, err
	}
	ah, err := f.broadcast(ctx, approve, autoSign)
	if err != nil {
		return FlowResult{}, err
	}
	// ERC-20 approve must be mined before createOrder (transferFrom) or it reverts.
	if ah != "" {
		if rw, ok := f.c.Signer.(ReceiptWaiter); ok {
			if err := rw.WaitForReceipt(ctx, ah, approve.ChainID); err != nil {
				return FlowResult{}, err
			}
		}
	}
	create, err := f.c.Checkout.TxIntent(ctx, checkoutID, "createOrder")
	if err != nil {
		return FlowResult{}, err
	}
	ch, err := f.broadcast(ctx, create, autoSign)
	if err != nil {
		return FlowResult{}, err
	}
	intents := []TxIntent{approve, create}
	hashes := nonEmpty(ah, ch)
	if ch == "" {
		return FlowResult{Intents: intents, TxHashes: hashes}, nil
	}
	if _, err := f.c.Checkout.Confirm(ctx, checkoutID, ch); err != nil {
		return FlowResult{}, err
	}
	read := func(ctx context.Context) (JSON, error) { return f.c.Checkout.Get(ctx, checkoutID) }
	done := func(s JSON) bool {
		if oid, ok := s["orderId"].(string); ok && oid != "" {
			return true
		}
		return s["status"] == "CONFIRMED"
	}
	session, err := WaitFor(ctx, read, done, wait.Timeout, wait.Interval)
	if err != nil {
		return FlowResult{}, err
	}
	var order JSON
	if oid, ok := session["orderId"].(string); ok && oid != "" {
		order, _ = f.c.Orders.Get(ctx, oid)
	}
	return FlowResult{Intents: intents, TxHashes: hashes, Result: order}, nil
}

// BuyListing instant-buys a listing straight through to a funded order.
func (f *Flows) BuyListing(ctx context.Context, listingID, clientAgentID string, autoSign bool, wait WaitOpts, extra JSON) (FlowResult, error) {
	body := JSON{"clientAgentId": clientAgentID}
	for k, v := range extra {
		body[k] = v
	}
	res, err := f.c.Listings.InstantBuy(ctx, listingID, body)
	if err != nil {
		return FlowResult{}, err
	}
	cid := firstString(res["checkoutId"], res["id"])
	if cid == "" {
		if co, ok := res["checkout"].(map[string]any); ok {
			cid, _ = co["id"].(string)
		}
	}
	if cid == "" {
		return FlowResult{}, fmt.Errorf("instant-buy did not return a checkout id")
	}
	return f.CompleteCheckout(ctx, cid, autoSign, wait)
}

// AcceptOrderAsProvider accepts a funded order on-chain (locks provider stake).
func (f *Flows) AcceptOrderAsProvider(ctx context.Context, orderID string, autoSign bool, wait WaitOpts) (FlowResult, error) {
	intent, err := f.c.Orders.ProviderAccept(ctx, orderID)
	if err != nil {
		return FlowResult{}, err
	}
	return f.single(ctx, orderID, intent, func(o JSON) bool { s := o["status"]; return s == "FUNDED" || s == "IN_PROGRESS" }, autoSign, wait)
}

// Deliver submits a delivery and polls to DELIVERED.
func (f *Flows) Deliver(ctx context.Context, orderID string, body JSON, autoSign bool, wait WaitOpts) (FlowResult, error) {
	intent, err := f.c.Orders.SubmitDelivery(ctx, orderID, body)
	if err != nil {
		return FlowResult{}, err
	}
	return f.single(ctx, orderID, intent, func(o JSON) bool { return o["status"] == "DELIVERED" }, autoSign, wait)
}

// Settle = buyer accepts the delivery (releaseEscrow).
func (f *Flows) Settle(ctx context.Context, orderID string, autoSign bool, wait WaitOpts) (FlowResult, error) {
	intent, err := f.c.Orders.Accept(ctx, orderID)
	if err != nil {
		return FlowResult{}, err
	}
	return f.single(ctx, orderID, intent, func(o JSON) bool { return o["status"] == "SETTLED" }, autoSign, wait)
}

// CancelExpired refunds an undelivered order past its deadline (permissionless).
func (f *Flows) CancelExpired(ctx context.Context, orderID string, autoSign bool, wait WaitOpts) (FlowResult, error) {
	intent, err := f.c.Orders.CancelExpired(ctx, orderID)
	if err != nil {
		return FlowResult{}, err
	}
	return f.single(ctx, orderID, intent, func(o JSON) bool { return o["status"] == "CANCELLED" }, autoSign, wait)
}

// ClaimAfterTimeout settles in the provider's favour after the challenge window.
func (f *Flows) ClaimAfterTimeout(ctx context.Context, orderID string, autoSign bool, wait WaitOpts) (FlowResult, error) {
	intent, err := f.c.Orders.ClaimAfterTimeout(ctx, orderID)
	if err != nil {
		return FlowResult{}, err
	}
	return f.single(ctx, orderID, intent, func(o JSON) bool { return o["status"] == "SETTLED" }, autoSign, wait)
}
