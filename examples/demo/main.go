// A read-only "consumer" walkthrough of the SDK against prod BSC (reads only).
// Mirrors the TypeScript examples/demo.ts. No secrets. Surfaces DX / business flow.
// Run: go run ./examples/demo   (or TERMIX_CHAIN=base go run ./examples/demo)
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	termix "github.com/TermiX-official/termix-sdk-go"
)

func h(title string) { fmt.Printf("\n── %s ──\n", title) }

func main() {
	ctx := context.Background()
	chain := os.Getenv("TERMIX_CHAIN")
	if chain == "" {
		chain = "bsc"
	}

	// 1. Spin up a client (warms /config/contracts).
	c := termix.New(termix.Options{Chain: chain, BaseURL: os.Getenv("TERMIX_BASE_URL")})
	cfg, err := c.Contracts(ctx)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	h("marketplace")
	fmt.Printf("chain=%s (#%d) fee=%dbps settle=%s\n",
		cfg.NetworkLabel, cfg.ChainID, cfg.ProtocolFeeBps, cfg.SettlementCurrency.Symbol)

	// 2. Network pulse.
	if stats, err := c.Stats.Network(ctx); err == nil {
		fmt.Printf("agents=%d live=%d vol=$%s block=%s\n",
			stats.VerifiedAgents, stats.LiveServices, stats.TotalVolumeUsd, stats.LatestBlock)
	}

	// 3. Browse the storefront — a buyer looking for a service.
	h("browse listings")
	page, err := c.Listings.List(ctx, url.Values{"pageSize": {"5"}, "sort": {"newest"}})
	if err == nil {
		fmt.Printf("showing %d of %d listings\n", len(page.Items), page.Total)
		for _, l := range page.Items {
			title, _ := l["title"].(string)
			fmt.Printf("  • %s\n", title)
		}
	}

	// 3b. Discover the taxonomy (categories + tags) used to filter.
	h("categories & tags")
	if cats, err := c.Listings.Categories(ctx); err == nil {
		labels := make([]string, 0, 4)
		for i, cat := range cats {
			if i >= 4 {
				break
			}
			labels = append(labels, cat.Label)
		}
		fmt.Printf("%d categories: %s …\n", len(cats), strings.Join(labels, " · "))
	}
	if tags, err := c.Listings.Tags(ctx, "a", 6); err == nil {
		names := make([]string, 0, len(tags))
		for _, t := range tags {
			names = append(names, t.Name)
		}
		fmt.Printf("tags matching \"a\": %s\n", strings.Join(names, ", "))
	}

	// 3c. A precise filtered search using the full filter set.
	h("filtered search (full filters)")
	filtered, err := c.Listings.List(ctx, url.Values{
		"query":          {"audit"},
		"instantBuyable": {"true"},
		"minRep":         {"50"},
		"sort":           {"reputation_desc"},
		"pageSize":       {"3"},
	})
	if err == nil {
		fmt.Printf("query=\"audit\" + instantBuyable + rep≥50 + sort=reputation_desc → %d results\n", filtered.Total)
	}

	// 4. Drill into one.
	if len(page.Items) > 0 {
		if id, ok := page.Items[0]["id"].(string); ok {
			h("listing detail")
			if detail, err := c.Listings.Get(ctx, id); err == nil {
				title, _ := detail["title"].(string)
				fmt.Printf("%s\n", title)
			}
		}
	}

	// 5. Explorer: who are the top agents?
	h("leaderboard")
	if board, err := c.Explorer.Leaderboard(ctx, url.Values{"window": {"7d"}, "limit": {"3"}}); err == nil {
		fmt.Printf("leaderboard fields: %d\n", len(board))
	}

	// 6. Money helper against the real settlement currency (zero float).
	h("money (no float)")
	dec := cfg.SettlementCurrency.Decimals
	if base, err := termix.ToBaseUnits("24.28", dec); err == nil {
		fmt.Printf("24.28 %s = %s base units; back = %s\n",
			cfg.SettlementCurrency.Symbol, base.String(), termix.FromBaseUnits(base, dec))
	}

	// 7. Signing mode — what a self-custody buyer gets vs an injected signer.
	h("signing modes")
	if c.Signer != nil {
		fmt.Println("  signer attached? yes (mode B — SDK signs, broadcasts, polls)")
	} else {
		fmt.Println("  signer attached? no (mode A — flows return unsigned intents)")
	}

	fmt.Println("\n✔ demo complete")
}
