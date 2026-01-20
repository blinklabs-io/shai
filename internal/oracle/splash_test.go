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

package oracle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSplashV1Parser(t *testing.T) {
	parser := NewSplashV1Parser()
	require.NotNil(t, parser, "NewSplashV1Parser returned nil")
}

func TestSplashParser_Protocol(t *testing.T) {
	parser := NewSplashV1Parser()
	assert.Equal(t, "splash-v1", parser.Protocol())
}

func TestSplashParser_ImplementsPoolParser(t *testing.T) {
	parser := NewSplashV1Parser()
	var _ PoolParser = parser // Compile-time check that SplashParser implements PoolParser
}
