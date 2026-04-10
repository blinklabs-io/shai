package config

import (
	"strings"
	"testing"
)

func TestMainnetCSwapProfileIsWired(t *testing.T) {
	t.Parallel()

	mainnetProfiles, ok := Profiles["mainnet"]
	if !ok {
		t.Fatal("missing mainnet profiles")
	}

	profile, ok := mainnetProfiles["cswap"]
	if !ok {
		t.Fatal("missing cswap profile")
	}

	if profile.Type != ProfileTypeOracle {
		t.Fatalf("unexpected profile type: got %v want %v", profile.Type, ProfileTypeOracle)
	}
	if profile.InterceptSlot == 0 {
		t.Fatal("cswap intercept slot must be non-zero")
	}
	if len(profile.InterceptHash) != 64 {
		t.Fatalf("unexpected intercept hash length: got %d want 64", len(profile.InterceptHash))
	}

	oracleCfg, ok := profile.Config.(OracleProfileConfig)
	if !ok {
		t.Fatalf("unexpected config type: %T", profile.Config)
	}
	if oracleCfg.Protocol != "cswap" {
		t.Fatalf("unexpected protocol: got %q want %q", oracleCfg.Protocol, "cswap")
	}
	if len(oracleCfg.PoolAddresses) == 0 {
		t.Fatal("cswap pool addresses must not be empty")
	}

	for i, poolAddress := range oracleCfg.PoolAddresses {
		if poolAddress.Address == "" {
			t.Fatalf("pool address %d must not be empty", i)
		}
		if !strings.HasPrefix(poolAddress.Address, "addr1") {
			t.Fatalf("pool address %d has unexpected format: %q", i, poolAddress.Address)
		}
	}
}
