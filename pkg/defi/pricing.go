package defi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/utils"
)

// defiLlamaBase is DefiLlama's keyless price API — generous limits, batch
// lookups across chains in one call, and returns token symbols as a bonus.
const defiLlamaBase = "https://coins.llama.fi"

// TokenPrice is one priced asset from DefiLlama.
type TokenPrice struct {
	PriceUSD   float64
	Symbol     string
	Decimals   int
	Confidence float64
}

// PriceClient fetches USD prices from DefiLlama.
type PriceClient struct {
	http *http.Client
}

// NewPriceClient builds a DefiLlama price client.
func NewPriceClient() (*PriceClient, error) {
	client, err := utils.CreateHTTPClient("", 15*time.Second)
	if err != nil {
		return nil, err
	}
	return &PriceClient{http: client}, nil
}

// NativeKey builds the DefiLlama key for a native coin via its CoinGecko id.
func NativeKey(coinGeckoID string) string { return "coingecko:" + coinGeckoID }

// TokenKey builds the DefiLlama key for a token contract/mint on a chain.
// Chain slugs match our chain names (ethereum, bsc, polygon, arbitrum, base,
// optimism, solana).
func TokenKey(chain, contract string) string { return chain + ":" + contract }

// Prices resolves USD prices for DefiLlama keys ("ethereum:0x…",
// "solana:<mint>", "coingecko:<id>"). Unknown assets are simply absent from
// the result — callers degrade to unpriced display.
func (p *PriceClient) Prices(ctx context.Context, keys []string) (map[string]TokenPrice, error) {
	out := map[string]TokenPrice{}
	if len(keys) == 0 {
		return out, nil
	}
	const batch = 80
	for i := 0; i < len(keys); i += batch {
		end := i + batch
		if end > len(keys) {
			end = len(keys)
		}
		url := fmt.Sprintf("%s/prices/current/%s", defiLlamaBase, strings.Join(keys[i:end], ","))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return out, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := p.http.Do(req)
		if err != nil {
			return out, fmt.Errorf("defillama: %w", err)
		}
		var raw struct {
			Coins map[string]struct {
				Price      float64 `json:"price"`
				Symbol     string  `json:"symbol"`
				Decimals   int     `json:"decimals"`
				Confidence float64 `json:"confidence"`
			} `json:"coins"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&raw)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return out, fmt.Errorf("defillama status %d", resp.StatusCode)
		}
		if decodeErr != nil {
			return out, fmt.Errorf("defillama decode: %w", decodeErr)
		}
		for key, c := range raw.Coins {
			out[strings.ToLower(key)] = TokenPrice{
				PriceUSD:   c.Price,
				Symbol:     strings.ToUpper(c.Symbol),
				Decimals:   c.Decimals,
				Confidence: c.Confidence,
			}
		}
	}
	return out, nil
}
