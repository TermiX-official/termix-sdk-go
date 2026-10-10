# Examples

Runnable examples for `termix-sdk-go`. All reads are against prod BSC and use **no secrets**.

| Example | Run | What it shows |
|---|---|---|
| `demo` | `go run ./examples/demo` | Consumer walkthrough: config, network stats, browse listings, categories & tags, filtered search, listing detail, leaderboard, money helper, signing-mode check. |
| `read-loop` | `go run ./cmd/read-loop` | Read-only closed loop used as a conformance check (config → listings → detail → explorer → stats). |

Override the chain or endpoint with env vars:

```bash
TERMIX_CHAIN=base go run ./examples/demo
TERMIX_BASE_URL=https://<your-host>/api/v1 go run ./examples/demo
```

Mode B (internal sign + broadcast) needs a `Signer`; see the top-level README for the signing modes.
