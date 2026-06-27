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

func TestMainnetSundaeSwapV1ProfileIsWired(t *testing.T) {
	t.Parallel()

	mainnetProfiles, ok := Profiles["mainnet"]
	if !ok {
		t.Fatal("missing mainnet profiles")
	}

	profile, ok := mainnetProfiles["sundaeswap-v1"]
	if !ok {
		t.Fatal("missing sundaeswap-v1 profile")
	}

	if profile.Type != ProfileTypeOracle {
		t.Fatalf("unexpected profile type: got %v want %v", profile.Type, ProfileTypeOracle)
	}
	if profile.InterceptSlot != 51337110 {
		t.Fatalf("unexpected intercept slot: got %d want %d", profile.InterceptSlot, 51337110)
	}
	if profile.InterceptHash != "d1423d7eb6fc87d7ad1a54e44dd8cb70483370877346f3178dd507d7609046c8" {
		t.Fatalf("unexpected intercept hash: got %q", profile.InterceptHash)
	}

	oracleCfg, ok := profile.Config.(OracleProfileConfig)
	if !ok {
		t.Fatalf("unexpected config type: %T", profile.Config)
	}
	if oracleCfg.Protocol != "sundaeswap-v1" {
		t.Fatalf(
			"unexpected protocol: got %q want %q",
			oracleCfg.Protocol,
			"sundaeswap-v1",
		)
	}
	if len(oracleCfg.PoolAddresses) != 1 {
		t.Fatalf("unexpected pool address count: got %d want 1", len(oracleCfg.PoolAddresses))
	}
	if !strings.HasPrefix(oracleCfg.PoolAddresses[0].Address, "addr1") {
		t.Fatalf("pool address has unexpected format: %q", oracleCfg.PoolAddresses[0].Address)
	}
}

func TestMainnetButaneProfileIsWired(t *testing.T) {
	t.Parallel()

	mainnetProfiles, ok := Profiles["mainnet"]
	if !ok {
		t.Fatal("missing mainnet profiles")
	}

	profile, ok := mainnetProfiles["butane"]
	if !ok {
		t.Fatal("missing butane profile")
	}
	if profile.Type != ProfileTypeSynthetics {
		t.Fatalf(
			"unexpected profile type: got %v want %v",
			profile.Type,
			ProfileTypeSynthetics,
		)
	}
	if profile.InterceptSlot == 0 {
		t.Fatal("butane intercept slot must be non-zero")
	}
	if strings.Trim(profile.InterceptHash, "0") == "" {
		t.Fatal("butane intercept hash must not be the zero hash")
	}
	if len(profile.InterceptHash) != 64 {
		t.Fatalf(
			"unexpected intercept hash length: got %d want 64",
			len(profile.InterceptHash),
		)
	}

	synthCfg, ok := profile.Config.(SyntheticsProfileConfig)
	if !ok {
		t.Fatalf("unexpected config type: %T", profile.Config)
	}
	if synthCfg.Protocol != "butane" {
		t.Fatalf("unexpected protocol: got %q want %q", synthCfg.Protocol, "butane")
	}
	if len(synthCfg.CDPAddresses) == 0 {
		t.Fatal("butane CDP addresses must not be empty")
	}
	if len(synthCfg.OracleAddresses) == 0 {
		t.Fatal("butane oracle addresses must not be empty")
	}
	for _, addr := range append(synthCfg.CDPAddresses, synthCfg.OracleAddresses...) {
		if !strings.HasPrefix(addr.Address, "addr1") {
			t.Fatalf("address has unexpected format: %q", addr.Address)
		}
	}
}
