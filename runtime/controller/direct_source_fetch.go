/*
Copyright 2026 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"
)

// FeatureGateDirectSourceFetch is a feature gate that enables direct fetching
// of source artifacts.
const FeatureGateDirectSourceFetch = "DirectSourceFetch"

// ErrDirectSourceFetchNotEnabled is returned when direct source fetch
// is attempted but not enabled.
var ErrDirectSourceFetchNotEnabled = fmt.Errorf(
	"%s feature gate is not enabled", FeatureGateDirectSourceFetch)

// SetFeatureGates sets the default values for the feature gates.
func SetFeatureGates(features map[string]bool) {
	// disabled by default.
	features[FeatureGateDirectSourceFetch] = false
}

// enableDirectSourceFetch enables the direct fetching of source artifacts.
var enableDirectSourceFetch bool

// EnableDirectSourceFetch enables the direct fetching of source artifacts.
func EnableDirectSourceFetch() {
	enableDirectSourceFetch = true
}

// DisableDirectSourceFetch disables the direct fetching of source artifacts.
func DisableDirectSourceFetch() {
	enableDirectSourceFetch = false
}

// IsDirectSourceFetchEnabled returns true if the direct source fetch
// feature gate is enabled.
func IsDirectSourceFetchEnabled() bool {
	return enableDirectSourceFetch
}
