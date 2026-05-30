package tools

import (
	"context"
	"fmt"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// earnProviderFn resolves a broker.EarnProvider for the given provider/account.
// Overridable in tests. Returns an error when the provider does not implement
// the EarnProvider interface (e.g. spot-only venues).
var earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
	p, err := broker.CreateProviderForAccount(providerID, account, cfg)
	if err != nil {
		return nil, fmt.Errorf("provider %q: %w", providerID, err)
	}
	ep, ok := p.(broker.EarnProvider)
	if !ok {
		return nil, fmt.Errorf("provider %q does not support earn/savings products", providerID)
	}
	_ = ctx
	return ep, nil
}

// earnProvider resolves a broker.EarnProvider via the (test-overridable) seam.
func earnProvider(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
	return earnProviderFn(ctx, cfg, providerID, account)
}
