package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/stellar/go-stellar-sdk/network"
)

// Global flag for quiet mode
var quietMode bool

// logInfo writes informational/progress messages to stderr.
// These messages are suppressed when --quiet flag is set.
func logInfo(format string, args ...interface{}) {
	if !quietMode {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// logError writes error messages to stderr.
// These messages are always shown, even in quiet mode.
func logError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}

// logWarning writes warning messages to stderr.
// These messages are always shown, even in quiet mode.
func logWarning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}

// ConfigSource represents where a configuration value came from
type ConfigSource struct {
	Value  string
	Source string // "flag", "env", "default"
}

// printStartupBanner displays configuration information before starting work.
// This helps users verify they're connecting to the correct network/RPC.
func printStartupBanner(command string, config map[string]ConfigSource, extraInfo map[string]string) {
	if quietMode {
		return
	}

	fmt.Fprintf(os.Stderr, "🚀 nebu %s\n", command)

	// Print main configuration
	for key, cfg := range config {
		sourceAnnotation := ""
		if cfg.Source != "default" {
			sourceAnnotation = fmt.Sprintf(" (from %s)", cfg.Source)
		}
		fmt.Fprintf(os.Stderr, "   %-8s %s%s\n", key+":", cfg.Value, sourceAnnotation)
	}

	// Print extra info (like Range)
	for key, value := range extraInfo {
		fmt.Fprintf(os.Stderr, "   %-8s %s\n", key+":", value)
	}

	fmt.Fprintf(os.Stderr, "\n")
}

// validateNetworkConfig checks for common network configuration mismatches.
// Returns an error with helpful guidance if a mismatch is detected.
func validateNetworkConfig(rpcURL, networkPass string) error {
	// Check for testnet RPC with mainnet network
	if strings.Contains(strings.ToLower(rpcURL), "testnet") {
		if networkPass == network.PublicNetworkPassphrase {
			return fmt.Errorf(`Network configuration mismatch detected:
  RPC URL:  %s (appears to be TESTNET)
  Network:  %s (MAINNET passphrase)

Did you mean to use --network testnet?

To fix:
  nebu [command] --rpc-url %s --network testnet ...

Or set environment:
  export NEBU_NETWORK=testnet`, rpcURL, networkPass, rpcURL)
		}
	}

	// Check for mainnet RPC with testnet network
	if strings.Contains(strings.ToLower(rpcURL), "mainnet") || strings.Contains(strings.ToLower(rpcURL), "pubnet") {
		if networkPass == network.TestNetworkPassphrase {
			return fmt.Errorf(`Network configuration mismatch detected:
  RPC URL:  %s (appears to be MAINNET)
  Network:  %s (TESTNET passphrase)

Did you mean to use --network mainnet?

To fix:
  nebu [command] --rpc-url %s --network mainnet ...

Or set environment:
  export NEBU_NETWORK=mainnet`, rpcURL, networkPass, rpcURL)
		}
	}

	return nil
}

// getEnvOrFlag returns a value from environment or flag, tracking the source.
func getEnvOrFlag(envKey, flagValue, defaultValue string) ConfigSource {
	if envValue := os.Getenv(envKey); envValue != "" {
		return ConfigSource{Value: envValue, Source: envKey}
	}
	if flagValue != "" && flagValue != defaultValue {
		return ConfigSource{Value: flagValue, Source: "--" + strings.ToLower(strings.ReplaceAll(envKey, "_", "-"))}
	}
	return ConfigSource{Value: defaultValue, Source: "default"}
}

// getEnvOrFlagInt gets integer configuration from environment or flag, tracking the source.
func getEnvOrFlagInt(envKey string, flagValue, defaultValue int) struct {
	Value  int
	Source string
} {
	type IntConfigSource struct {
		Value  int
		Source string
	}

	if val := os.Getenv(envKey); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return IntConfigSource{Value: intVal, Source: envKey}
		}
	}
	if flagValue != defaultValue {
		return IntConfigSource{Value: flagValue, Source: "--" + strings.ToLower(strings.ReplaceAll(envKey, "_", "-"))}
	}
	return IntConfigSource{Value: defaultValue, Source: "default"}
}

// maskSecret returns a masked version of a secret, showing only last 4 characters.
func maskSecret(secret string) string {
	if len(secret) <= 4 {
		return "***"
	}
	return "***" + secret[len(secret)-4:]
}

// getAuthDisplay formats auth token for display (masked).
func getAuthDisplay(authHeader string) string {
	if authHeader == "" {
		return "None"
	}

	// Parse "Authorization: Bearer xxx" or "Api-Key xxx"
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) == 2 {
		authType := parts[0]
		token := parts[1]
		return fmt.Sprintf("%s (%s)", authType, maskSecret(token))
	}

	return maskSecret(authHeader)
}

// getNetworkDisplay returns a human-readable network name.
func getNetworkDisplay(passphrase string) string {
	switch passphrase {
	case network.PublicNetworkPassphrase:
		return "Public Global Stellar Network ; September 2015"
	case network.TestNetworkPassphrase:
		return "Test SDF Network ; September 2015"
	default:
		return passphrase
	}
}

// normalizeNetworkPassphrase converts common network aliases to full passphrases.
// Accepts: "mainnet", "testnet", or full passphrases.
func normalizeNetworkPassphrase(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "mainnet", "main", "public":
		return network.PublicNetworkPassphrase
	case "testnet", "test":
		return network.TestNetworkPassphrase
	default:
		return input
	}
}

// getAuthHeader returns the authorization header value from flags or environment.
// Priority: --rpc-header flag > NEBU_RPC_AUTH env var
func getAuthHeader(rpcHeaders []string) string {
	// First check explicit --rpc-header flags
	for _, header := range rpcHeaders {
		// Parse "Authorization: Value" format
		parts := strings.SplitN(header, ":", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == "Authorization" {
			return strings.TrimSpace(parts[1])
		}
	}

	// Fall back to NEBU_RPC_AUTH environment variable
	if authEnv := os.Getenv("NEBU_RPC_AUTH"); authEnv != "" {
		return authEnv
	}

	return ""
}

// getAuthConfig returns a ConfigSource for the auth configuration.
func getAuthConfig(authHeader string) ConfigSource {
	if authHeader == "" {
		return ConfigSource{Value: "None", Source: "default"}
	}

	// Determine source
	source := "NEBU_RPC_AUTH"
	if os.Getenv("NEBU_RPC_AUTH") == "" {
		source = "--rpc-header"
	}

	return ConfigSource{
		Value:  getAuthDisplay(authHeader),
		Source: source,
	}
}
