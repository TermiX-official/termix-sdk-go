package termix

import (
	"context"
	"time"
)

// TxIntent is the unsigned on-chain intent returned by every write/money endpoint.
type TxIntent struct {
	ID         string `json:"id"`
	AccountID  string `json:"accountId"`
	OrderID    string `json:"orderId,omitempty"`
	Action     string `json:"action"`
	ChainID    int    `json:"chainId"`
	Contract   string `json:"contract"`
	CallData   string `json:"callData"`
	Value      string `json:"value"`
	Status     string `json:"status"`
	TxHash     string `json:"txHash,omitempty"`
	NonceKey   string `json:"nonceKey"`
	CreatedAt  string `json:"createdAt,omitempty"`
}

// TxRequest is a wallet-ready transaction built from a TxIntent.
type TxRequest struct {
	ChainID int
	To      string
	Data    string
	Value   string
}

// Signer is the minimal signing port. Adapters (go-ethereum, KMS, ...) implement it.
type Signer interface {
	Address(ctx context.Context) (string, error)
	SignMessage(ctx context.Context, message string) (string, error)
	SendTransaction(ctx context.Context, tx TxRequest) (string, error)
}

// ReceiptWaiter is optional; flows use it to order dependent txs (approve -> createOrder).
type ReceiptWaiter interface {
	WaitForReceipt(ctx context.Context, txHash string, chainID int) error
}

func intentToTx(i TxIntent) TxRequest {
	v := i.Value
	if v == "0" {
		v = ""
	}
	return TxRequest{ChainID: i.ChainID, To: i.Contract, Data: i.CallData, Value: v}
}

func broadcastIntent(ctx context.Context, s Signer, i TxIntent) (string, error) {
	return s.SendTransaction(ctx, intentToTx(i))
}

// WaitFor polls a fresh read until done — platform state is event-driven, not from
// the broadcast result.
func WaitFor[T any](ctx context.Context, read func(context.Context) (T, error), done func(T) bool, timeout, interval time.Duration) (T, error) {
	var zero T
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	if interval == 0 {
		interval = 2 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		v, err := read(ctx)
		if err != nil {
			return zero, err
		}
		if done(v) {
			return v, nil
		}
		if time.Now().After(deadline) {
			return zero, &NetworkError{Msg: "WaitFor: predicate not satisfied before timeout"}
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(interval):
		}
	}
}
