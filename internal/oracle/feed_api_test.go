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
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/blinklabs-io/shai/oraclefeed"
	"github.com/stretchr/testify/require"
)

const feedAPIOrcfaxFixture = "d8799fd8799f4d4345522f4144412d5553442f331b0000019edc8ac6a2d8799f1a00027e051a000f4240ffffd8799f581c3c12f6735ef87655c5b27bced3f828d857d0a27fd20f2cda18ebf2fbffff"

func TestFeedAPIReportsStaleSourceAndUnavailablePrice(t *testing.T) {
	tracker := oraclefeed.NewTracker()
	applyFeedAPIFixture(t, tracker)
	api := NewFeedAPI(tracker)
	api.now = func() time.Time {
		return time.Date(2026, 7, 23, 18, 4, 4, 0, time.UTC)
	}

	mux := http.NewServeMux()
	api.RegisterHandlers(mux)

	statusResponse := httptest.NewRecorder()
	mux.ServeHTTP(
		statusResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/feeds", nil),
	)
	require.Equal(t, http.StatusOK, statusResponse.Code)
	var statuses struct {
		Feeds []oraclefeed.SourceStatus `json:"feeds"`
		Count int                       `json:"count"`
	}
	require.NoError(t, json.Unmarshal(statusResponse.Body.Bytes(), &statuses))
	require.Equal(t, 2, statuses.Count)
	require.False(t, statuses.Feeds[1].Fresh)
	require.Contains(t, statuses.Feeds[1].Error, "stale")

	priceResponse := httptest.NewRecorder()
	mux.ServeHTTP(
		priceResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/prices/ada-usd", nil),
	)
	require.Equal(t, http.StatusServiceUnavailable, priceResponse.Code)
	require.Contains(t, priceResponse.Body.String(), "no fresh authenticated")
}

func TestFeedAPIReturnsFreshAuthenticatedPrice(t *testing.T) {
	tracker := oraclefeed.NewTracker()
	applyFeedAPIFixture(t, tracker)
	api := NewFeedAPI(tracker)
	api.now = func() time.Time {
		return time.Date(2026, 6, 18, 22, 0, 0, 0, time.UTC)
	}

	response := httptest.NewRecorder()
	api.HandleADAUSD(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/prices/ada-usd", nil),
	)
	require.Equal(t, http.StatusOK, response.Code)
	var observation oraclefeed.Observation
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &observation))
	require.Equal(t, oraclefeed.SourceOrcfax, observation.Source)
	require.Equal(t, uint64(163333), observation.Numerator)
	require.Equal(t, uint64(1_000_000), observation.Denominator)
}

func applyFeedAPIFixture(t *testing.T, tracker *oraclefeed.Tracker) {
	t.Helper()
	datum, err := hex.DecodeString(feedAPIOrcfaxFixture)
	require.NoError(t, err)
	_, matched, err := tracker.Apply(oraclefeed.UTxO{
		Address: oraclefeed.OrcfaxADAUSDAddress,
		Assets: []oraclefeed.Asset{{
			PolicyID: oraclefeed.OrcfaxFeedPolicyID,
			Quantity: 1,
		}},
		Datum:   datum,
		TxHash:  "a16b451afbb11198a0f179d79c5dc6ebe863e2c348783e088a3d8c0f5952b91f",
		TxIndex: 0,
	})
	require.True(t, matched)
	require.NoError(t, err)
}
