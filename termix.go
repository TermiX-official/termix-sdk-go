// Package termix is the Go SDK for the TermiX (AACP) marketplace platform API —
// a business-flow wrapper, not a thin HTTP client.
//
// Status: scaffold (M1). The full port of the TypeScript SDK's flows/resources
// layers, SIWE auth, and dual-mode signing lands in milestone M5; the TypeScript
// package (@termix-it/platform-sdk) is the spec baseline. Money is string/big.Int
// (never float); on-chain signing (mode B) uses go-ethereum in an optional subpackage.
package termix

// Version of the SDK.
const Version = "0.0.0"

// Chain is a single platform deployment (separate backend + DB + contracts).
type Chain struct {
	ChainID int
	Label   string
	Host    string // API host without the /api/v1 suffix
}

// Chains is the authoritative registry (verified against the skill registry + live prod).
var Chains = map[string]Chain{
	"bsc":  {ChainID: 56, Label: "BNB Chain", Host: "https://platform-backend.prod.termix.live"},
	"base": {ChainID: 8453, Label: "Base", Host: "https://platform-backend-base.prod.termix.live"},
	"rh":   {ChainID: 4663, Label: "Robinhood Chain", Host: "https://platform-backend-rh.prod.termix.live"},
}

// DefaultChain is used when no chain is specified.
const DefaultChain = "bsc"
