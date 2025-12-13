package contract_invocation

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// ConvertScValToJSON converts an xdr.ScVal to a JSON-serializable interface
func ConvertScValToJSON(val xdr.ScVal) (interface{}, error) {
	switch val.Type {
	case xdr.ScValTypeScvBool:
		return val.MustB(), nil

	case xdr.ScValTypeScvVoid:
		return nil, nil

	case xdr.ScValTypeScvU32:
		return val.MustU32(), nil

	case xdr.ScValTypeScvI32:
		return val.MustI32(), nil

	case xdr.ScValTypeScvU64:
		return val.MustU64(), nil

	case xdr.ScValTypeScvI64:
		return val.MustI64(), nil

	case xdr.ScValTypeScvU128:
		u128 := val.MustU128()
		return map[string]interface{}{
			"hi": u128.Hi,
			"lo": u128.Lo,
		}, nil

	case xdr.ScValTypeScvI128:
		i128 := val.MustI128()
		return map[string]interface{}{
			"hi": i128.Hi,
			"lo": i128.Lo,
		}, nil

	case xdr.ScValTypeScvU256:
		u256 := val.MustU256()
		return map[string]interface{}{
			"hi_hi": u256.HiHi,
			"hi_lo": u256.HiLo,
			"lo_hi": u256.LoHi,
			"lo_lo": u256.LoLo,
		}, nil

	case xdr.ScValTypeScvI256:
		i256 := val.MustI256()
		return map[string]interface{}{
			"hi_hi": i256.HiHi,
			"hi_lo": i256.HiLo,
			"lo_hi": i256.LoHi,
			"lo_lo": i256.LoLo,
		}, nil

	case xdr.ScValTypeScvBytes:
		bytes := val.MustBytes()
		return hex.EncodeToString(bytes), nil

	case xdr.ScValTypeScvString:
		return string(val.MustStr()), nil

	case xdr.ScValTypeScvSymbol:
		return string(val.MustSym()), nil

	case xdr.ScValTypeScvVec:
		vec := val.MustVec()
		if vec == nil {
			return []interface{}{}, nil
		}
		result := make([]interface{}, len(*vec))
		for i, item := range *vec {
			converted, err := ConvertScValToJSON(item)
			if err != nil {
				return nil, err
			}
			result[i] = converted
		}
		return result, nil

	case xdr.ScValTypeScvMap:
		scMap := val.MustMap()
		if scMap == nil {
			return map[string]interface{}{}, nil
		}
		result := make(map[string]interface{})
		for i, entry := range *scMap {
			keyConverted, err := ConvertScValToJSON(entry.Key)
			if err != nil {
				return nil, err
			}

			// Convert key to string for map
			keyStr := fmt.Sprintf("%v", keyConverted)
			if keyConverted == nil {
				keyStr = fmt.Sprintf("key_%d", i)
			}

			valConverted, err := ConvertScValToJSON(entry.Val)
			if err != nil {
				return nil, err
			}
			result[keyStr] = valConverted
		}
		return result, nil

	case xdr.ScValTypeScvAddress:
		address := val.MustAddress()
		switch address.Type {
		case xdr.ScAddressTypeScAddressTypeAccount:
			accountID := address.MustAccountId()
			return accountID.Address(), nil
		case xdr.ScAddressTypeScAddressTypeContract:
			contractID := address.MustContractId()
			encoded, err := strkey.Encode(strkey.VersionByteContract, contractID[:])
			if err != nil {
				return nil, err
			}
			return encoded, nil
		}

	case xdr.ScValTypeScvLedgerKeyContractInstance:
		return "LedgerKeyContractInstance", nil

	case xdr.ScValTypeScvLedgerKeyNonce:
		nonce := val.MustNonceKey()
		return map[string]interface{}{
			"nonce": nonce.Nonce,
		}, nil

	case xdr.ScValTypeScvContractInstance:
		return "ContractInstance", nil

	case xdr.ScValTypeScvTimepoint:
		return val.MustTimepoint(), nil

	case xdr.ScValTypeScvDuration:
		return val.MustDuration(), nil
	}

	return map[string]interface{}{
		"type":  val.Type.String(),
		"value": "unsupported",
	}, nil
}

// ConvertScValToString converts an xdr.ScVal to a JSON string
func ConvertScValToString(val xdr.ScVal) string {
	converted, err := ConvertScValToJSON(val)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	jsonBytes, err := json.Marshal(converted)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}

	return string(jsonBytes)
}

// GetFunctionNameFromScVal extracts a function name from an ScVal (typically a symbol)
func GetFunctionNameFromScVal(val xdr.ScVal) string {
	if val.Type == xdr.ScValTypeScvSymbol {
		return string(val.MustSym())
	}
	return ""
}

// ExtractFunctionName extracts the function name from a contract invocation
func ExtractFunctionName(invokeContract xdr.InvokeContractArgs) string {
	// Primary method: Use FunctionName field directly
	if len(invokeContract.FunctionName) > 0 {
		return string(invokeContract.FunctionName)
	}

	// Fallback: Check first argument if it contains function name
	if len(invokeContract.Args) > 0 {
		functionName := GetFunctionNameFromScVal(invokeContract.Args[0])
		if functionName != "" {
			return functionName
		}
	}

	return "unknown"
}

// ExtractArguments extracts and converts all function arguments
func ExtractArguments(args []xdr.ScVal) []string {
	result := make([]string, 0, len(args))

	for _, arg := range args {
		result = append(result, ConvertScValToString(arg))
	}

	return result
}
