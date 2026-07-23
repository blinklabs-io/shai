// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dex

import (
	"math"
	"testing"

	"github.com/blinklabs-io/shai/common"
)

func TestTVL(t *testing.T) {
	tests := []struct {
		name string
		x, y uint64
		want uint64
	}{
		{name: "simple sum", x: 100, y: 250, want: 350},
		{name: "zero", x: 0, y: 0, want: 0},
		{
			name: "overflow saturates to MaxUint64",
			x:    math.MaxUint64 - 10,
			y:    100,
			want: math.MaxUint64,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &PoolState{
				AssetX: common.AssetAmount{Amount: tc.x},
				AssetY: common.AssetAmount{Amount: tc.y},
			}
			if got := p.TVL(); got != tc.want {
				t.Errorf("TVL() = %d, want %d", got, tc.want)
			}
		})
	}
}
