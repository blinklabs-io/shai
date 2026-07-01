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

func TestPreprodMinswapV2ProfileIsWired(t *testing.T) {
	t.Parallel()

	profile := requireProfile(t, "preprod", "minswap-v2")

	if profile.Type != ProfileTypeOracle {
		t.Fatalf("unexpected profile type: got %v want %v", profile.Type, ProfileTypeOracle)
	}
	if profile.InterceptSlot != 62910357 {
		t.Fatalf("unexpected intercept slot: got %d want %d", profile.InterceptSlot, 62910357)
	}
	if profile.InterceptHash != "42d965bbc11668723b3bc3741a969c2750d2df0d16714a0af63b4ef2ee221eb1" {
		t.Fatalf("unexpected intercept hash: got %q", profile.InterceptHash)
	}

	oracleCfg, ok := profile.Config.(OracleProfileConfig)
	if !ok {
		t.Fatalf("unexpected config type: %T", profile.Config)
	}
	if oracleCfg.Protocol != "minswap-v2" {
		t.Fatalf("unexpected protocol: got %q want %q", oracleCfg.Protocol, "minswap-v2")
	}
	if len(oracleCfg.PoolAddresses) != 1 {
		t.Fatalf("unexpected pool address count: got %d want 1", len(oracleCfg.PoolAddresses))
	}
	if oracleCfg.PoolAddresses[0].Address != "addr_test1wrtt4xm4p84vse3g3l6swtf2rqs943t0w39ustwdszxt3lsyrt40u" {
		t.Fatalf("unexpected pool address: %q", oracleCfg.PoolAddresses[0].Address)
	}
	if len(oracleCfg.InputRefs) != 1 {
		t.Fatalf("unexpected input ref count: got %d want 1", len(oracleCfg.InputRefs))
	}
	if oracleCfg.InputRefs[0] != (ProfileConfigInputRef{
		TxId:      "9f30b1c3948a009ceebda32d0b1d25699674b2eaf8b91ef029a43bfc1073ce28",
		OutputIdx: 0,
	}) {
		t.Fatalf("unexpected input ref: %#v", oracleCfg.InputRefs[0])
	}
}

func TestPreviewSundaeSwapV3ProfileIsWired(t *testing.T) {
	t.Parallel()

	profile := requireProfile(t, "preview", "sundaeswap-v3")

	if profile.Type != ProfileTypeOracle {
		t.Fatalf("unexpected profile type: got %v want %v", profile.Type, ProfileTypeOracle)
	}
	if profile.InterceptSlot != 48535234 {
		t.Fatalf("unexpected intercept slot: got %d want %d", profile.InterceptSlot, 48535234)
	}
	if profile.InterceptHash != "311319fd4889453ddbba6fde8b085cd1ec8d7d341e6128c003ad8cbbfbf09043" {
		t.Fatalf("unexpected intercept hash: got %q", profile.InterceptHash)
	}

	oracleCfg, ok := profile.Config.(OracleProfileConfig)
	if !ok {
		t.Fatalf("unexpected config type: %T", profile.Config)
	}
	if oracleCfg.Protocol != "sundaeswap-v3" {
		t.Fatalf("unexpected protocol: got %q want %q", oracleCfg.Protocol, "sundaeswap-v3")
	}
	if len(oracleCfg.PoolAddresses) != 1 {
		t.Fatalf("unexpected pool address count: got %d want 1", len(oracleCfg.PoolAddresses))
	}
	if oracleCfg.PoolAddresses[0].Address != "addr_test1xpz2r6ednav2m48tryet6qzgu6segl59u0ly7v54dggsg9xvy7vq4p2hl6wm9jdvpgn80ax3xpkm7yrgnxphtrct3klq005j2r" {
		t.Fatalf("unexpected pool address: %q", oracleCfg.PoolAddresses[0].Address)
	}
	if len(oracleCfg.InputRefs) != 2 {
		t.Fatalf("unexpected input ref count: got %d want 2", len(oracleCfg.InputRefs))
	}
	if oracleCfg.InputRefs[0] != (ProfileConfigInputRef{
		TxId:      "92ec2274938de291d3837b7facf9eddfaed57cd6ff97e26af57cb7a9978e3887",
		OutputIdx: 0,
	}) {
		t.Fatalf("unexpected order input ref: %#v", oracleCfg.InputRefs[0])
	}
	if oracleCfg.InputRefs[1] != (ProfileConfigInputRef{
		TxId:      "8036a88a61427262aba964a42d0b9924739ffc3214de9a07c54b5a09af7f0d7d",
		OutputIdx: 0,
	}) {
		t.Fatalf("unexpected pool input ref: %#v", oracleCfg.InputRefs[1])
	}
}

func requireProfile(t *testing.T, network string, name string) Profile {
	t.Helper()

	networkProfiles, ok := Profiles[network]
	if !ok {
		t.Fatalf("missing %s profiles", network)
	}
	profile, ok := networkProfiles[name]
	if !ok {
		t.Fatalf("missing %s profile", name)
	}
	return profile
}
