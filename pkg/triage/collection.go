// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package triage

import (
	"context"
	"fmt"
	"time"

	"github.com/google/triage-party/pkg/hubbub"
	"k8s.io/klog/v2"
)

// Collection represents a fully loaded YAML configuration
type Collection struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description,omitempty"`
	RuleIDs      []string `yaml:"rules"`
	Dedup        bool     `yaml:"dedup,omitempty"`
	Hidden       bool     `yaml:"hidden,omitempty"`
	UsedForStats bool     `yaml:"used_for_statistics,omitempty"`
}

// The result of Execute
type CollectionResult struct {
	Time time.Time

	RuleResults []*RuleResult

	Total             int
	TotalPullRequests int
	TotalIssues       int

	AvgAge             time.Duration
	AvgCurrentHold     time.Duration
	AvgAccumulatedHold time.Duration

	TotalAgeDays             float64
	TotalCurrentHoldDays     float64
	TotalAccumulatedHoldDays float64
}

// ExecuteCollection executes a collection.
func (p *Party) ExecuteCollection(ctx context.Context, s Collection, newerThan time.Time) (*CollectionResult, error) {
	klog.V(1).Infof("executing collection %q: %s", s.ID, s.RuleIDs)
	start := time.Now()

	os := []*RuleResult{}
	seen := map[string]*Rule{}
	seenRule := map[string]bool{}
	var latestInput time.Time

	for _, tid := range s.RuleIDs {
		if seenRule[tid] {
			klog.Errorf("collection %q has a duplicate rule: %q - ignoring", s.ID, tid)
			continue
		}

		seenRule[tid] = true

		t, err := p.LookupRule(tid)
		if err != nil {
			return nil, err
		}

		ro, err := p.ExecuteRule(ctx, t, seen, newerThan)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %v", t.Name, err)
		}

		if ro.LatestInput.After(latestInput) {
			latestInput = ro.LatestInput
		}

		os = append(os, ro)
	}

	r := SummarizeCollectionResult(os)
	r.Time = newerThan

	// If we are lucky, our results may be newer than we asked for!
	if latestInput.After(r.Time) {
		r.Time = latestInput
	}

	klog.V(1).Infof("collection %q took %s", s.ID, time.Since(start))
	return r, nil
}

// SummarizeCollectionResult adds together statistics about collection results {
func SummarizeCollectionResult(os []*RuleResult) *CollectionResult {
	klog.V(1).Infof("Summarizing collection result with %d rules...", len(os))

	r := &CollectionResult{}

	for _, oc := range os {
		r.Total += len(oc.Items)
		if oc.Rule.Type == hubbub.PullRequest {
			r.TotalPullRequests += len(oc.Items)
		} else {
			r.TotalIssues += len(oc.Items)
		}

		r.RuleResults = append(r.RuleResults, oc)

		r.TotalAgeDays += oc.TotalAgeDays
		r.TotalCurrentHoldDays += oc.TotalCurrentHoldDays
		r.TotalAccumulatedHoldDays += oc.TotalAccumulatedHoldDays

	}

	if r.Total == 0 {
		return r
	}

	r.AvgAge = avgDayDuration(r.TotalAgeDays, r.Total)
	r.AvgCurrentHold = avgDayDuration(r.TotalCurrentHoldDays, r.Total)
	r.AvgAccumulatedHold = avgDayDuration(r.TotalAccumulatedHoldDays, r.Total)
	return r
}

func avgDayDuration(total float64, count int) time.Duration {
	return time.Duration(int64(total/float64(count)*24)) * time.Hour
}

// ListCollections a fully resolved collections
func (p *Party) ListCollections() ([]Collection, error) {
	return p.collections, nil
}

// Return a fully resolved collection
func (p *Party) LookupCollection(id string) (Collection, error) {
	for _, s := range p.collections {
		if s.ID == id {
			return s, nil
		}
	}
	return Collection{}, fmt.Errorf("%q not found", id)
}
