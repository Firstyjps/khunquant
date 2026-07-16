// Package defi provides read-only DeFi wallet tracking across EVM chains and
// Solana via public JSON-RPC endpoints — no API keys, no heavy web3 SDKs.
package defi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/utils"
)

// EVMChain describes one EVM-compatible chain reachable over public JSON-RPC.
// RPCs are tried in order — public endpoints fail intermittently, so every
// chain gets at least one fallback.
type EVMChain struct {
	Name              string
	RPCs              []string
	NativeSymbol      string
	NativeCoinGeckoID string
	CoinGeckoPlatform string // platform slug for token price lookups
}

// DefaultEVMChains are the built-in chains with keyless public RPCs.
var DefaultEVMChains = map[string]EVMChain{
	"ethereum": {
		Name: "ethereum", RPCs: []string{"https://ethereum-rpc.publicnode.com", "https://eth.llamarpc.com", "https://cloudflare-eth.com"},
		NativeSymbol: "ETH", NativeCoinGeckoID: "ethereum", CoinGeckoPlatform: "ethereum",
	},
	"bsc": {
		Name: "bsc", RPCs: []string{"https://bsc-rpc.publicnode.com", "https://bsc-dataseed.binance.org"},
		NativeSymbol: "BNB", NativeCoinGeckoID: "binancecoin", CoinGeckoPlatform: "binance-smart-chain",
	},
	"polygon": {
		Name: "polygon", RPCs: []string{"https://polygon-bor-rpc.publicnode.com", "https://polygon-rpc.com"},
		NativeSymbol: "POL", NativeCoinGeckoID: "polygon-ecosystem-token", CoinGeckoPlatform: "polygon-pos",
	},
	"arbitrum": {
		Name: "arbitrum", RPCs: []string{"https://arbitrum-one-rpc.publicnode.com", "https://arb1.arbitrum.io/rpc"},
		NativeSymbol: "ETH", NativeCoinGeckoID: "ethereum", CoinGeckoPlatform: "arbitrum-one",
	},
	"base": {
		Name: "base", RPCs: []string{"https://base-rpc.publicnode.com", "https://mainnet.base.org"},
		NativeSymbol: "ETH", NativeCoinGeckoID: "ethereum", CoinGeckoPlatform: "base",
	},
	"optimism": {
		Name: "optimism", RPCs: []string{"https://optimism-rpc.publicnode.com", "https://mainnet.optimism.io"},
		NativeSymbol: "ETH", NativeCoinGeckoID: "ethereum", CoinGeckoPlatform: "optimistic-ethereum",
	},
}

// DefaultWatchTokens are well-known ERC-20 contracts checked on every wallet
// (plain RPC cannot enumerate holdings — additional tokens can be watched per
// wallet or passed to the balance tool).
var DefaultWatchTokens = map[string][]string{
	"ethereum": {
		"0xdAC17F958D2ee523a2206206994597C13D831ec7", // USDT
		"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", // USDC
		"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", // WETH
		"0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599", // WBTC
		"0x6B175474E89094C44Da98b954EedeAC495271d0F", // DAI
	},
	"bsc": {
		"0x55d398326f99059fF775485246999027B3197955", // USDT (BSC)
		"0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", // USDC (BSC)
		"0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c", // WBNB
	},
	"polygon": {
		"0xc2132D05D31c914a87C6611C10748AEb04B58e8F", // USDT
		"0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359", // USDC
		"0x7ceB23fD6bC0adD59E62ac25578270cFf1b9f619", // WETH
	},
	"arbitrum": {
		"0xFd086bC7CD5C481DCC9C85ebE478A1C0b69FCbb9", // USDT
		"0xaf88d065e77c8cC2239327C5EDb3A432268e5831", // USDC
		"0x82aF49447D8a07e3bd95BD0d56f35241523fBab1", // WETH
	},
	"base": {
		"0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", // USDC
		"0x4200000000000000000000000000000000000006", // WETH
	},
	"optimism": {
		"0x94b008aA00579c1307B0EF2c499aD98a8ce58e58", // USDT
		"0x0b2C639c533813f4Aa9D7837CAf62653d097Ff85", // USDC
		"0x4200000000000000000000000000000000000006", // WETH
	},
}

var evmAddressRe = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

// IsEVMAddress reports whether s looks like a 20-byte hex EVM address.
func IsEVMAddress(s string) bool { return evmAddressRe.MatchString(s) }

// EVMClient talks JSON-RPC to a single EVM chain.
type EVMClient struct {
	chain EVMChain
	http  *http.Client
}

// NewEVMClient builds a client for a named default chain.
func NewEVMClient(chainName string) (*EVMClient, error) {
	chain, ok := DefaultEVMChains[strings.ToLower(chainName)]
	if !ok {
		var names []string
		for n := range DefaultEVMChains {
			names = append(names, n)
		}
		return nil, fmt.Errorf("unknown EVM chain %q (supported: %s + solana)", chainName, strings.Join(names, ", "))
	}
	client, err := utils.CreateHTTPClient("", 20*time.Second)
	if err != nil {
		return nil, err
	}
	return &EVMClient{chain: chain, http: client}, nil
}

func (c *EVMClient) Chain() EVMChain { return c.chain }

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      int    `json:"id"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// call posts a JSON-RPC request, trying each configured endpoint in order
// until one answers without error (public RPCs fail intermittently).
func (c *EVMClient) call(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1})
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, rpc := range c.chain.RPCs {
		result, err := postRPC(ctx, c.http, rpc, body)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("%s rpc (all endpoints failed): %w", c.chain.Name, lastErr)
}

func postRPC(ctx context.Context, client *http.Client, url string, body []byte) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("rpc decode: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

func hexToBig(hexStr string) (*big.Int, error) {
	s := strings.TrimPrefix(strings.Trim(hexStr, `"`), "0x")
	if s == "" {
		return big.NewInt(0), nil
	}
	n, ok := new(big.Int).SetString(s, 16)
	if !ok {
		return nil, fmt.Errorf("bad hex quantity %q", hexStr)
	}
	return n, nil
}

func bigToFloat(n *big.Int, decimals int) float64 {
	f := new(big.Float).SetInt(n)
	div := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	out, _ := new(big.Float).Quo(f, div).Float64()
	return out
}

// NativeBalance returns the native-coin balance (ETH/BNB/POL...) of address.
func (c *EVMClient) NativeBalance(ctx context.Context, address string) (float64, error) {
	if !IsEVMAddress(address) {
		return 0, fmt.Errorf("invalid EVM address %q", address)
	}
	raw, err := c.call(ctx, "eth_getBalance", address, "latest")
	if err != nil {
		return 0, err
	}
	n, err := hexToBig(string(raw))
	if err != nil {
		return 0, err
	}
	return bigToFloat(n, 18), nil
}

// ABI selectors (first 4 bytes of keccak256 of the signature).
const (
	selBalanceOf = "0x70a08231"
	selDecimals  = "0x313ce567"
	selSymbol    = "0x95d89b41"
)

func (c *EVMClient) ethCall(ctx context.Context, to, data string) (string, error) {
	raw, err := c.call(ctx, "eth_call", map[string]string{"to": to, "data": data}, "latest")
	if err != nil {
		return "", err
	}
	return strings.Trim(string(raw), `"`), nil
}

// ERC20Balance returns the token balance of wallet, scaled by the token's decimals.
func (c *EVMClient) ERC20Balance(ctx context.Context, token, wallet string) (float64, int, error) {
	if !IsEVMAddress(token) || !IsEVMAddress(wallet) {
		return 0, 0, fmt.Errorf("invalid token/wallet address")
	}
	padded := strings.Repeat("0", 24) + strings.ToLower(strings.TrimPrefix(wallet, "0x"))
	res, err := c.ethCall(ctx, token, selBalanceOf+padded)
	if err != nil {
		return 0, 0, err
	}
	n, err := hexToBig(res)
	if err != nil {
		return 0, 0, err
	}
	decimals := 18
	if dres, derr := c.ethCall(ctx, token, selDecimals); derr == nil {
		if d, e := hexToBig(dres); e == nil && d.Int64() > 0 && d.Int64() <= 36 {
			decimals = int(d.Int64())
		}
	}
	return bigToFloat(n, decimals), decimals, nil
}

// ERC20Symbol best-effort reads the token's symbol (handles both ABI-encoded
// string and legacy bytes32 returns). Falls back to a shortened address.
func (c *EVMClient) ERC20Symbol(ctx context.Context, token string) string {
	res, err := c.ethCall(ctx, token, selSymbol)
	if err != nil {
		return ShortAddress(token)
	}
	if sym := decodeABIString(res); sym != "" {
		return sym
	}
	return ShortAddress(token)
}

// decodeABIString decodes an eth_call return that is either a dynamic ABI
// string (offset+len+data) or a null-padded bytes32.
func decodeABIString(hexStr string) string {
	s := strings.TrimPrefix(hexStr, "0x")
	raw := make([]byte, 0, len(s)/2)
	for i := 0; i+1 < len(s); i += 2 {
		var b byte
		fmt.Sscanf(s[i:i+2], "%02x", &b)
		raw = append(raw, b)
	}
	clean := func(b []byte) string {
		out := strings.TrimRight(string(b), "\x00")
		for _, r := range out {
			if r < 0x20 || r > 0x7e {
				return ""
			}
		}
		return strings.TrimSpace(out)
	}
	// Dynamic string: 32-byte offset, 32-byte length, then data.
	if len(raw) >= 64 {
		length := new(big.Int).SetBytes(raw[32:64]).Int64()
		if length > 0 && 64+int(length) <= len(raw) {
			if s := clean(raw[64 : 64+length]); s != "" {
				return s
			}
		}
	}
	// bytes32 fallback.
	if len(raw) >= 32 {
		return clean(raw[:32])
	}
	return ""
}

// ShortAddress renders 0xabcd…1234 for display.
func ShortAddress(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return addr[:6] + "…" + addr[len(addr)-4:]
}
