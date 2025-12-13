// Package amount_filter provides the core filtering logic for the amount-filter processor.
// This logic is shared between CLI and gRPC implementations.
package amount_filter

import "strconv"

// Filter encapsulates the amount filtering configuration and logic.
type Filter struct {
	MinAmount int64
	MaxAmount int64
	AssetCode string
}

// NewFilter creates a new amount filter with the given parameters.
func NewFilter(minAmount, maxAmount int64, assetCode string) *Filter {
	return &Filter{
		MinAmount: minAmount,
		MaxAmount: maxAmount,
		AssetCode: assetCode,
	}
}

// FilterEvent applies the amount filter logic to an event.
// Returns the event if it passes the filters, nil if it should be filtered out.
func (f *Filter) FilterEvent(event map[string]interface{}) map[string]interface{} {
	// Extract the event data from protojson format
	// Events can be: transfer, mint, burn, clawback, fee
	var eventData map[string]interface{}
	var ok bool

	// Try each event type
	for _, eventType := range []string{"transfer", "mint", "burn", "clawback", "fee"} {
		if eventData, ok = event[eventType].(map[string]interface{}); ok {
			break
		}
	}

	if eventData == nil {
		return nil // Not a recognized event type
	}

	// Get amount field from the event data
	amountStr, ok := eventData["amount"].(string)
	if !ok {
		return nil // No amount field, filter out
	}

	// Parse amount
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		return nil // Invalid amount, filter out
	}

	// Check minimum
	if f.MinAmount > 0 && amount < f.MinAmount {
		return nil
	}

	// Check maximum
	if f.MaxAmount > 0 && amount > f.MaxAmount {
		return nil
	}

	// Check asset if specified
	if f.AssetCode != "" {
		asset, ok := eventData["asset"].(map[string]interface{})
		if !ok {
			return nil
		}

		// Check for issued asset
		issuedAsset, ok := asset["issuedAsset"].(map[string]interface{})
		if ok {
			// Issued asset - check asset code
			code, ok := issuedAsset["assetCode"].(string)
			if !ok || code != f.AssetCode {
				return nil
			}
		} else if nativeAsset, ok := asset["native"].(bool); ok && nativeAsset {
			// Native asset - check if looking for native/XLM
			if f.AssetCode != "native" && f.AssetCode != "XLM" {
				return nil
			}
		} else {
			return nil // Unknown asset format
		}
	}

	// Passed all filters
	return event
}
