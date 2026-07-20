package entity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/utils"
)

// Esplora (mempool.space / blockstream.info) REST API — keyless, no JS
// challenge, same schema on both hosts, so each acts as the other's fallback.
var defaultEsploraBases = []string{
	"https://mempool.space/api",
	"https://blockstream.info/api",
}

var btcAddressRe = regexp.MustCompile(`^(1|3)[a-km-zA-HJ-NP-Z1-9]{25,34}$|^bc1[a-z0-9]{11,87}$`)

// IsBTCAddress reports whether s looks like a mainnet Bitcoin address
// (legacy, P2SH, or bech32).
func IsBTCAddress(s string) bool { return btcAddressRe.MatchString(s) }

// BTCClient reads Bitcoin address data from Esplora-compatible APIs.
type BTCClient struct {
	bases []string
	http  *http.Client
}

// NewBTCClient builds a client over the default Esplora hosts.
func NewBTCClient() (*BTCClient, error) {
	client, err := utils.CreateHTTPClient("", 20*time.Second)
	if err != nil {
		return nil, err
	}
	return &BTCClient{bases: defaultEsploraBases, http: client}, nil
}

// NewBTCClientWithBases builds a client over custom base URLs (tests).
func NewBTCClientWithBases(bases []string) (*BTCClient, error) {
	c, err := NewBTCClient()
	if err != nil {
		return nil, err
	}
	c.bases = bases
	return c, nil
}

// get tries each base in order until one returns HTTP 200.
func (c *BTCClient) get(ctx context.Context, path string, out any) error {
	var lastErr error
	for _, base := range c.bases {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("%s%s: status %d", base, path, resp.StatusCode)
			continue
		}
		if err := json.Unmarshal(body, out); err != nil {
			lastErr = fmt.Errorf("%s%s: decode: %w", base, path, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("all esplora hosts failed: %w", lastErr)
}

type esploraStats struct {
	FundedSum int64 `json:"funded_txo_sum"`
	SpentSum  int64 `json:"spent_txo_sum"`
	TxCount   int   `json:"tx_count"`
}

// AddressSummary is the confirmed balance + activity of one BTC address.
type AddressSummary struct {
	Address    string
	BalanceBTC float64 // confirmed + mempool delta
	TxCount    int     // confirmed tx count
}

// Summary fetches balance and tx count for a BTC address.
func (c *BTCClient) Summary(ctx context.Context, address string) (AddressSummary, error) {
	var raw struct {
		Address      string       `json:"address"`
		ChainStats   esploraStats `json:"chain_stats"`
		MempoolStats esploraStats `json:"mempool_stats"`
	}
	if err := c.get(ctx, "/address/"+address, &raw); err != nil {
		return AddressSummary{}, err
	}
	sats := (raw.ChainStats.FundedSum - raw.ChainStats.SpentSum) +
		(raw.MempoolStats.FundedSum - raw.MempoolStats.SpentSum)
	return AddressSummary{
		Address:    address,
		BalanceBTC: float64(sats) / 1e8,
		TxCount:    raw.ChainStats.TxCount,
	}, nil
}

type esploraTx struct {
	TxID   string `json:"txid"`
	Status struct {
		Confirmed bool  `json:"confirmed"`
		BlockTime int64 `json:"block_time"`
	} `json:"status"`
	Vin []struct {
		Prevout struct {
			Address string `json:"scriptpubkey_address"`
			Value   int64  `json:"value"`
		} `json:"prevout"`
	} `json:"vin"`
	Vout []struct {
		Address string `json:"scriptpubkey_address"`
		Value   int64  `json:"value"`
	} `json:"vout"`
}

// Counterparty is the other side of a transfer, by total value moved.
type Counterparty struct {
	Address  string
	Label    string // filled in by the caller via Store.LabelFor
	ValueBTC float64
}

// Transfer is one transaction seen from the tracked address's perspective.
type Transfer struct {
	TxID           string
	Time           time.Time
	Confirmed      bool
	AmountBTC      float64 // signed: + inflow, − outflow (net for this address)
	Counterparties []Counterparty
}

// RecentTransfers returns up to limit recent transactions for the address,
// each reduced to a signed net amount plus the top counterparties on the
// other side of the flow.
func (c *BTCClient) RecentTransfers(ctx context.Context, address string, limit int) ([]Transfer, error) {
	if limit <= 0 || limit > 25 {
		limit = 10 // esplora returns max 25 per page; keep responses chat-sized
	}
	var txs []esploraTx
	if err := c.get(ctx, "/address/"+address+"/txs", &txs); err != nil {
		return nil, err
	}
	if len(txs) > limit {
		txs = txs[:limit]
	}

	out := make([]Transfer, 0, len(txs))
	for _, tx := range txs {
		var inSats, outSats int64 // funds leaving / arriving at `address`
		others := map[string]int64{}
		for _, vin := range tx.Vin {
			if vin.Prevout.Address == address {
				inSats += vin.Prevout.Value
			}
		}
		for _, vout := range tx.Vout {
			if vout.Address == address {
				outSats += vout.Value
			}
		}
		net := outSats - inSats // + = inflow to address
		if net >= 0 {
			// Inflow: counterparties are the senders (inputs not ours).
			for _, vin := range tx.Vin {
				if a := vin.Prevout.Address; a != "" && a != address {
					others[a] += vin.Prevout.Value
				}
			}
		} else {
			// Outflow: counterparties are the receivers (outputs not ours).
			for _, vout := range tx.Vout {
				if a := vout.Address; a != "" && a != address {
					others[a] += vout.Value
				}
			}
		}
		out = append(out, Transfer{
			TxID:           tx.TxID,
			Time:           time.Unix(tx.Status.BlockTime, 0).UTC(),
			Confirmed:      tx.Status.Confirmed,
			AmountBTC:      float64(net) / 1e8,
			Counterparties: topCounterparties(others, 3),
		})
	}
	return out, nil
}

func topCounterparties(m map[string]int64, n int) []Counterparty {
	out := make([]Counterparty, 0, len(m))
	for a, v := range m {
		out = append(out, Counterparty{Address: a, ValueBTC: float64(v) / 1e8})
	}
	// Highest value first; deterministic tiebreak by address.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ValueBTC > out[i].ValueBTC ||
				(out[j].ValueBTC == out[i].ValueBTC && out[j].Address < out[i].Address) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}
