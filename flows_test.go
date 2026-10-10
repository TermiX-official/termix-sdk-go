package termix

import (
	"context"
	"testing"
	"time"
)

type mockSigner struct {
	events []string
	n      int
}

func (m *mockSigner) Address(ctx context.Context) (string, error) { return "0xabc", nil }
func (m *mockSigner) SignMessage(ctx context.Context, msg string) (string, error) {
	return "0xsig", nil
}
func (m *mockSigner) SendTransaction(ctx context.Context, tx TxRequest) (string, error) {
	m.n++
	h := "0xhash" + itoa(m.n)
	m.events = append(m.events, "send:"+h)
	return h, nil
}
func (m *mockSigner) WaitForReceipt(ctx context.Context, hash string, chainID int) error {
	m.events = append(m.events, "receipt:"+hash)
	return nil
}

var fast = WaitOpts{Timeout: 2 * time.Second, Interval: 10 * time.Millisecond}

func TestCompleteCheckoutModeBOrdering(t *testing.T) {
	var order []string
	srv, _ := testServer(map[string]routeFn{
		"POST /api/v1/checkout/c1/tx-intent": func(n int) (int, any) {
			return 200, map[string]any{"id": "i", "action": "x", "chainId": 56, "contract": "0xee", "callData": "0xaa", "value": "0", "nonceKey": "k"}
		},
		"POST /api/v1/checkout/c1/confirm": func(n int) (int, any) { return 200, map[string]any{"ok": true} },
		"GET /api/v1/checkout/c1":          func(n int) (int, any) { return 200, map[string]any{"id": "c1", "status": "CONFIRMED", "orderId": "o1"} },
		"GET /api/v1/orders/o1":            func(n int) (int, any) { return 200, map[string]any{"id": "o1", "status": "SETTLED"} },
	})
	defer srv.Close()
	_ = order
	signer := &mockSigner{}
	c := New(Options{BaseURL: srv.URL, Signer: signer})
	res, err := c.Flows.CompleteCheckout(context.Background(), "c1", true, fast)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TxHashes) != 2 {
		t.Fatalf("want 2 tx hashes, got %d", len(res.TxHashes))
	}
	if res.Result["id"] != "o1" {
		t.Fatalf("want order o1, got %v", res.Result)
	}
	// approve receipt awaited between the two sends
	sends := 0
	receiptAfterFirstSend := false
	for _, e := range signer.events {
		if len(e) >= 5 && e[:5] == "send:" {
			sends++
		}
		if e[:8] == "receipt:" && sends == 1 {
			receiptAfterFirstSend = true
		}
	}
	if !receiptAfterFirstSend {
		t.Errorf("approve receipt not awaited before createOrder: %v", signer.events)
	}
}

func TestCompleteCheckoutModeANoBroadcast(t *testing.T) {
	srv, _ := testServer(map[string]routeFn{
		"POST /api/v1/checkout/c1/tx-intent": func(n int) (int, any) {
			return 200, map[string]any{"id": "i", "action": "x", "chainId": 56, "contract": "0xee", "callData": "0xaa", "value": "0", "nonceKey": "k"}
		},
	})
	defer srv.Close()
	c := New(Options{BaseURL: srv.URL}) // no signer
	res, err := c.Flows.CompleteCheckout(context.Background(), "c1", false, fast)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Intents) != 2 || len(res.TxHashes) != 0 {
		t.Errorf("mode A: want 2 intents and 0 hashes, got %d/%d", len(res.Intents), len(res.TxHashes))
	}
}
