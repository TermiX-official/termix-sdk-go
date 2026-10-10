// M5 acceptance: read-only closed loop against prod BSC (no secrets).
// Run: go run ./cmd/read-loop   (or TERMIX_CHAIN=base go run ./cmd/read-loop)
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	termix "github.com/TermiX-official/termix-sdk-go"
)

func main() {
	ctx := context.Background()
	chain := os.Getenv("TERMIX_CHAIN")
	if chain == "" {
		chain = "bsc"
	}
	c := termix.New(termix.Options{Chain: chain, BaseURL: os.Getenv("TERMIX_BASE_URL")})
	fmt.Printf("\nTermiX Platform SDK (Go) — read-loop (chain=%s)\n\n", chain)

	fails := 0
	check := func(label string, ok bool, detail any) {
		mark := "✓"
		if !ok {
			mark = "✗"
			fails++
		}
		fmt.Printf("  %s %s -> %v\n", mark, label, detail)
	}
	must := func(err error) {
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
	}

	cfg, err := c.Contracts(ctx)
	must(err)
	check("chainId", cfg.ChainID > 0, cfg.ChainID)
	check("escrow address", cfg.Contracts["escrow"].Address != "", cfg.Contracts["escrow"].Address)
	dec := cfg.SettlementCurrency.Decimals
	check("settlement decimals", dec > 0, dec)

	stats, err := c.Stats.Network(ctx)
	must(err)
	check("latestBlock", stats.LatestBlock != "", stats.LatestBlock)

	q := url.Values{}
	q.Set("pageSize", "3")
	lst, err := c.Listings.List(ctx, q)
	must(err)
	check("items", len(lst.Items) > 0, len(lst.Items))
	check("pagination total", lst.Total >= 0, lst.Total)

	if len(lst.Items) > 0 {
		id, _ := lst.Items[0]["id"].(string)
		detail, err := c.Listings.Get(ctx, id)
		must(err)
		did, _ := detail["id"].(string)
		check("detail id matches", did == id, did)
	}

	ag, err := c.Explorer.Agents(ctx, q)
	must(err)
	check("explorer items", len(ag.Items) > 0, len(ag.Items))

	base, err := termix.ToBaseUnits("24.28", dec)
	must(err)
	rt := termix.FromBaseUnits(base, dec)
	check("money round-trip (no float)", rt == "24.28", map[string]string{"in": "24.28", "out": rt})

	fmt.Println()
	if fails == 0 {
		fmt.Println("ALL PASS")
	} else {
		fmt.Printf("%d FAILED\n", fails)
		os.Exit(1)
	}
}
