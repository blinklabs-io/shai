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
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/blinklabs-io/shai/oraclefeed"
)

// FeedAPI exposes authenticated observations collected from local chain sync.
type FeedAPI struct {
	tracker *oraclefeed.Tracker
	now     func() time.Time
}

func NewFeedAPI(tracker *oraclefeed.Tracker) *FeedAPI {
	return &FeedAPI{tracker: tracker, now: time.Now}
}

func (a *FeedAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/feeds", a.HandleListFeeds)
	mux.HandleFunc("GET /api/v1/prices/ada-usd", a.HandleADAUSD)
}

func (a *FeedAPI) HandleListFeeds(w http.ResponseWriter, _ *http.Request) {
	statuses := a.tracker.Sources(a.now().UTC())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"feeds": statuses,
		"count": len(statuses),
	})
}

func (a *FeedAPI) HandleADAUSD(w http.ResponseWriter, _ *http.Request) {
	now := a.now().UTC()
	observation, err := a.tracker.ADAUSD(now)
	if err != nil {
		if errors.Is(err, oraclefeed.ErrNoFreshObservation) ||
			errors.Is(err, oraclefeed.ErrDivergentObservations) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   err.Error(),
				"sources": a.tracker.Sources(a.now().UTC()),
			})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		oraclefeed.Observation
		Price      float64 `json:"price"`
		AgeSeconds int64   `json:"ageSeconds"`
		Validation string  `json:"validation"`
	}{
		Observation: observation,
		Price:       observation.Float64(),
		AgeSeconds:  int64(now.Sub(observation.ObservedAt).Seconds()),
		Validation:  "verified",
	})
}
