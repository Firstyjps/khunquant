package defi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/utils"
)

// SolanaRPC is the keyless public mainnet endpoint.
const SolanaRPC = "https://api.mainnet-beta.solana.com"

// splTokenProgram is the SPL Token program id used to enumerate token accounts.
const splTokenProgram = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"

var solanaAddressRe = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)

// IsSolanaAddress reports whether s looks like a base58 Solana pubkey.
func IsSolanaAddress(s string) bool { return solanaAddressRe.MatchString(s) }

// SolanaClient talks JSON-RPC to Solana mainnet.
type SolanaClient struct {
	rpc  string
	http *http.Client
}

// NewSolanaClient builds a client for the public mainnet RPC.
func NewSolanaClient() (*SolanaClient, error) {
	client, err := utils.CreateHTTPClient("", 20*time.Second)
	if err != nil {
		return nil, err
	}
	return &SolanaClient{rpc: SolanaRPC, http: client}, nil
}

func (c *SolanaClient) call(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpc, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("solana rpc: %w", err)
	}
	defer resp.Body.Close()
	var out rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("solana rpc decode: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("solana rpc error %d: %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

// NativeBalance returns the SOL balance of address.
func (c *SolanaClient) NativeBalance(ctx context.Context, address string) (float64, error) {
	if !IsSolanaAddress(address) {
		return 0, fmt.Errorf("invalid Solana address %q", address)
	}
	raw, err := c.call(ctx, "getBalance", address)
	if err != nil {
		return 0, err
	}
	var res struct {
		Value uint64 `json:"value"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, err
	}
	return float64(res.Value) / 1e9, nil
}

// SPLBalance is one SPL token holding.
type SPLBalance struct {
	Mint     string
	Amount   float64
	Decimals int
}

// SPLBalances enumerates every SPL token account owned by address — unlike
// EVM, Solana RPC supports full holdings discovery keylessly.
func (c *SolanaClient) SPLBalances(ctx context.Context, address string) ([]SPLBalance, error) {
	if !IsSolanaAddress(address) {
		return nil, fmt.Errorf("invalid Solana address %q", address)
	}
	raw, err := c.call(ctx, "getTokenAccountsByOwner",
		address,
		map[string]string{"programId": splTokenProgram},
		map[string]string{"encoding": "jsonParsed"},
	)
	if err != nil {
		return nil, err
	}
	var res struct {
		Value []struct {
			Account struct {
				Data struct {
					Parsed struct {
						Info struct {
							Mint        string `json:"mint"`
							TokenAmount struct {
								UIAmount float64 `json:"uiAmount"`
								Decimals int     `json:"decimals"`
							} `json:"tokenAmount"`
						} `json:"info"`
					} `json:"parsed"`
				} `json:"data"`
			} `json:"account"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, err
	}
	var out []SPLBalance
	for _, v := range res.Value {
		info := v.Account.Data.Parsed.Info
		if info.TokenAmount.UIAmount <= 0 {
			continue
		}
		out = append(out, SPLBalance{
			Mint:     info.Mint,
			Amount:   info.TokenAmount.UIAmount,
			Decimals: info.TokenAmount.Decimals,
		})
	}
	return out, nil
}
