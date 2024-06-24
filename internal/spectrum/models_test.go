package spectrum_test

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/shai/internal/spectrum"

	"github.com/blinklabs-io/gouroboros/cbor"
)

var testDefs = []struct {
	cborHex       string
	poolConfigObj spectrum.PoolConfig
}{
	{
		cborHex: "d8799fd8799f581ca22ebe57c45d0be3ba4bebca5a9d4877b42d7fd872f3d740414fa1244c414144415f4144415f4e4654ffd8799f4040ffd8799f581c8fef2d34078659493ce161a6c7fba4b56afefa8535296a5743f695874441414441ffd8799f581cad951d57c5b1e4e0bfc503dc7e4080bdb89db179e6853a271ca1b1294b414144415f4144415f4c51ff1903e59f581c13d68210e8e25f69ea14c5f381d010eb3fac36afa9faa240509b81b9ff1b00000004a817c800ff",
		poolConfigObj: spectrum.PoolConfig{
			Nft: spectrum.AssetClass{
				PolicyId: testModelsDecodeHex("a22ebe57c45d0be3ba4bebca5a9d4877b42d7fd872f3d740414fa124"),
				Name:     testModelsDecodeHex("414144415f4144415f4e4654"),
			},
			X: spectrum.AssetClass{
				PolicyId: []byte{},
				Name:     []byte{},
			},
			Y: spectrum.AssetClass{
				PolicyId: testModelsDecodeHex("8fef2d34078659493ce161a6c7fba4b56afefa8535296a5743f69587"),
				Name:     testModelsDecodeHex("41414441"),
			},
			Lq: spectrum.AssetClass{
				PolicyId: testModelsDecodeHex("ad951d57c5b1e4e0bfc503dc7e4080bdb89db179e6853a271ca1b129"),
				Name:     testModelsDecodeHex("414144415f4144415f4c51"),
			},
			FeeNum: 0x3e5,
			AdminPolicy: [][]byte{
				testModelsDecodeHex("13d68210e8e25f69ea14c5f381d010eb3fac36afa9faa240509b81b9"),
			},
			LqBound: 0x4a817c800,
		},
	},
	{
		cborHex: "d8799fd8799f581c0891dc34b96a4d705e11f0a1d14a3eb582623bffa38a9052a9dc1f204a41495f4144415f4e4654ffd8799f4040ffd8799f581cd542ad1dc269ae601125e8259cb8427c6b37c1d3569d10441df0291f424149ffd8799f581c895af34f5ef7d077d35d2b48a35df9b2c893e4621332a014ad62cb934941495f4144415f4c51ff1903e58000ff",
		poolConfigObj: spectrum.PoolConfig{
			Nft: spectrum.AssetClass{
				// AI_ADA_NFT, policy_id = 0891dc34b96a4d705e11f0a1d14a3eb582623bffa38a9052a9dc1f20
				PolicyId: testModelsDecodeHex("0891dc34b96a4d705e11f0a1d14a3eb582623bffa38a9052a9dc1f20"),
				Name:     []byte("AI_ADA_NFT"),
			},
			X: spectrum.AssetClass{
				PolicyId: []byte{},
				Name:     []byte{},
			},
			Y: spectrum.AssetClass{
				// AI, policy_id = d542ad1dc269ae601125e8259cb8427c6b37c1d3569d10441df0291f
				PolicyId: testModelsDecodeHex("d542ad1dc269ae601125e8259cb8427c6b37c1d3569d10441df0291f"),
				Name:     []byte("AI"),
			},
			Lq: spectrum.AssetClass{
				// AI_ADA_LQ, policy_id = 895af34f5ef7d077d35d2b48a35df9b2c893e4621332a014ad62cb93
				PolicyId: testModelsDecodeHex("895af34f5ef7d077d35d2b48a35df9b2c893e4621332a014ad62cb93"),
				Name:     []byte("AI_ADA_LQ"),
			},
			FeeNum:      997,
			AdminPolicy: [][]byte{},
			LqBound:     0,
		},
	},
}

func testModelsDecodeHex(hexData string) []byte {
	data, err := hex.DecodeString(hexData)
	if err != nil {
		panic(fmt.Sprintf("failed to decode hex: %s", err))
	}
	return data[:]
}

func TestPoolConfigEncodeDecode(t *testing.T) {
	for _, testDef := range testDefs {
		tmpCborData, err := hex.DecodeString(testDef.cborHex)
		if err != nil {
			t.Fatalf("failed to decode test CBOR hex: %s", err)
		}
		var tmpPoolConfig spectrum.PoolConfig
		if _, err := cbor.Decode(tmpCborData, &tmpPoolConfig); err != nil {
			t.Fatalf("failed to decode test CBOR: %s", err)
		}
		// Set CBOR in test def object for proper comparison
		tmpTestPoolConfig := testDef.poolConfigObj
		tmpTestPoolConfig.SetCbor(tmpCborData)
		if !reflect.DeepEqual(tmpPoolConfig, tmpTestPoolConfig) {
			t.Fatalf(
				"CBOR did not decode to expected object\n  got:    %s\n  wanted: %s",
				tmpPoolConfig.String(),
				tmpTestPoolConfig.String(),
			)
		}
	}
}

func TestPoolConfigEncode(t *testing.T) {
	for _, testDef := range testDefs {
		tmpCbor, err := cbor.Encode(&testDef.poolConfigObj)
		if err != nil {
			t.Fatalf("failed to encode test object to CBOR: %s", err)
		}
		tmpCborHex := hex.EncodeToString(tmpCbor)
		if tmpCborHex != strings.ToLower(testDef.cborHex) {
			t.Fatalf(
				"test object did not encode to expected CBOR\n  got:    %s\n  wanted: %s",
				tmpCborHex,
				strings.ToLower(testDef.cborHex),
			)
		}
	}
}
