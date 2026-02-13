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

package controller_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/fluxcd/pkg/runtime/controller"
)

func TestFeatureGateDirectSourceFetch(t *testing.T) {
	t.Run("constant has correct value", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(controller.FeatureGateDirectSourceFetch).To(Equal("DirectSourceFetch"))
	})

	t.Run("error message contains feature gate name", func(t *testing.T) {
		g := NewWithT(t)
		g.Expect(controller.ErrDirectSourceFetchNotEnabled.Error()).To(ContainSubstring("DirectSourceFetch"))
		g.Expect(controller.ErrDirectSourceFetchNotEnabled.Error()).To(ContainSubstring("feature gate is not enabled"))
	})
}

func TestSetFeatureGates(t *testing.T) {
	t.Run("sets DirectSourceFetch to false by default", func(t *testing.T) {
		g := NewWithT(t)

		features := make(map[string]bool)
		controller.SetFeatureGates(features)

		g.Expect(features).To(HaveKey(controller.FeatureGateDirectSourceFetch))
		g.Expect(features[controller.FeatureGateDirectSourceFetch]).To(BeFalse())
	})

	t.Run("does not override existing features", func(t *testing.T) {
		g := NewWithT(t)

		features := map[string]bool{
			"ExistingFeature": true,
		}
		controller.SetFeatureGates(features)

		g.Expect(features).To(HaveKey("ExistingFeature"))
		g.Expect(features["ExistingFeature"]).To(BeTrue())
		g.Expect(features).To(HaveKey(controller.FeatureGateDirectSourceFetch))
		g.Expect(features[controller.FeatureGateDirectSourceFetch]).To(BeFalse())
	})
}

func TestEnableDisableDirectSourceFetch(t *testing.T) {
	// Ensure clean state after each test
	t.Cleanup(func() {
		controller.DisableDirectSourceFetch()
	})

	t.Run("disabled by default", func(t *testing.T) {
		g := NewWithT(t)
		controller.DisableDirectSourceFetch() // Reset state
		g.Expect(controller.IsDirectSourceFetchEnabled()).To(BeFalse())
	})

	t.Run("can be enabled", func(t *testing.T) {
		g := NewWithT(t)
		controller.EnableDirectSourceFetch()
		g.Expect(controller.IsDirectSourceFetchEnabled()).To(BeTrue())
	})

	t.Run("can be disabled after being enabled", func(t *testing.T) {
		g := NewWithT(t)
		controller.EnableDirectSourceFetch()
		g.Expect(controller.IsDirectSourceFetchEnabled()).To(BeTrue())

		controller.DisableDirectSourceFetch()
		g.Expect(controller.IsDirectSourceFetchEnabled()).To(BeFalse())
	})

	t.Run("enable is idempotent", func(t *testing.T) {
		g := NewWithT(t)
		controller.EnableDirectSourceFetch()
		controller.EnableDirectSourceFetch()
		g.Expect(controller.IsDirectSourceFetchEnabled()).To(BeTrue())
	})

	t.Run("disable is idempotent", func(t *testing.T) {
		g := NewWithT(t)
		controller.DisableDirectSourceFetch()
		controller.DisableDirectSourceFetch()
		g.Expect(controller.IsDirectSourceFetchEnabled()).To(BeFalse())
	})
}
