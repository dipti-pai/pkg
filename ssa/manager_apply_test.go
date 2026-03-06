/*
Copyright 2021 Stefan Prodan
Copyright 2021 The Flux authors

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

package ssa

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ssaerrors "github.com/fluxcd/pkg/ssa/errors"
	"github.com/fluxcd/pkg/ssa/jsondiff"
	"github.com/fluxcd/pkg/ssa/normalize"
	"github.com/fluxcd/pkg/ssa/utils"
)

func TestApply(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("apply")
	objects, err := readManifest("testdata/test1.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")

	configMapName, configMap := getFirstObject(objects, "ConfigMap", id)

	t.Run("creates objects in order", func(t *testing.T) {
		// create objects
		changeSet, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}

		// expected created order
		sort.Sort(SortableUnstructureds(objects))
		var expected []string
		for _, object := range objects {
			expected = append(expected, utils.FmtUnstructured(object))
		}

		// verify the change set contains only created actions
		var output []string
		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(entry.Action, CreatedAction); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
			output = append(output, entry.Subject)
		}

		// verify the change set contains all objects in the right order
		if diff := cmp.Diff(expected, output); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	})

	t.Run("does not apply unchanged objects", func(t *testing.T) {
		// no-op apply
		changeSet, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}

		// verify the change set contains only unchanged actions
		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(UnchangedAction, entry.Action); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s\n%v", diff, changeSet)
			}
		}
	})

	t.Run("applies only changed objects", func(t *testing.T) {
		// update a value in the configmap
		err = unstructured.SetNestedField(configMap.Object, "val", "data", "key")
		if err != nil {
			t.Fatal(err)
		}

		// apply changes
		changeSet, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}

		// verify the change set contains the configured action only for the configmap
		for _, entry := range changeSet.Entries {
			if entry.Subject == configMapName {
				if diff := cmp.Diff(ConfiguredAction, entry.Action); diff != "" {
					t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
				}
			} else {
				if diff := cmp.Diff(UnchangedAction, entry.Action); diff != "" {
					t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
				}
			}
		}

		// get the configmap from cluster
		configMapClone := configMap.DeepCopy()
		err = manager.client.Get(ctx, client.ObjectKeyFromObject(configMapClone), configMapClone)
		if err != nil {
			t.Fatal(err)
		}

		// get data value from the in-cluster configmap
		val, _, err := unstructured.NestedFieldCopy(configMapClone.Object, "data", "key")
		if err != nil {
			t.Fatal(err)
		}

		// verify the configmap was updated in cluster with the right data value
		if diff := cmp.Diff(val, "val"); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	})
}

func TestApplyAllStaged_PartialFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	id := generateName("test-staged-fail")
	objects, err := readManifest("testdata/test12.yaml", id)
	if err != nil {
		t.Fatal(err)
	}
	partialFailureID := fmt.Sprintf("%s-partial-failure", id)

	// This test requires a non-cluster-admin client to
	// make sure ClusterRoleBinding and ClusterRole objects can be
	// applied in the same call to ApplyAllStaged.
	partialFailureNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: partialFailureID,
		},
	}
	if err := manager.client.Create(ctx, partialFailureNS); err != nil {
		t.Fatal(err)
	}
	partialFailureSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      partialFailureID,
			Namespace: partialFailureID,
		},
	}
	if err := manager.client.Create(ctx, partialFailureSA); err != nil {
		t.Fatal(err)
	}
	partialFailureCR := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: partialFailureID,
		},
		Rules: []rbacv1.PolicyRule{
			// For applying the manifests from testdata/test12.yaml
			{
				APIGroups: []string{"", "rbac.authorization.k8s.io", "storage.k8s.io"},
				Resources: []string{"namespaces", "clusterroles", "clusterrolebindings", "storageclasses"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			// The ServiceAccount must have all the permissions it is indirectly granting
			// through the ClusterRole manifest from testdata/test12.yaml
			{
				APIGroups: []string{"apps"},
				Resources: []string{"*"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}
	if err := manager.client.Create(ctx, partialFailureCR); err != nil {
		t.Fatal(err)
	}
	partialFailureCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: partialFailureID,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     partialFailureCR.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      partialFailureSA.Name,
			Namespace: partialFailureSA.Namespace,
		}},
	}
	if err := manager.client.Create(ctx, partialFailureCRB); err != nil {
		t.Fatal(err)
	}

	// Copy test suite manager and modify the client to one that isn't
	// cluster-admin.
	partialFailureManager := *manager
	cfg := rest.CopyConfig(cfg)
	cfg.Impersonate = rest.ImpersonationConfig{
		UserName: fmt.Sprintf("system:serviceaccount:%s:%s", partialFailureSA.Namespace, partialFailureSA.Name),
	}
	client, err := client.New(cfg, client.Options{
		Mapper: restMapper,
	})
	if err != nil {
		t.Fatal(err)
	}
	partialFailureManager.client = client

	partialFailureManager.SetOwnerLabels(objects, "app1", "default")

	_, crb := getFirstObject(objects, "ClusterRoleBinding", id)

	t.Run("creates objects in order", func(t *testing.T) {
		// create objects
		changeSet, err := partialFailureManager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}

		// expected created order
		expected := []string{
			fmt.Sprintf("Namespace/%s", id),
			fmt.Sprintf("ClusterRole/%s", id),
			fmt.Sprintf("StorageClass/%s", id),
			fmt.Sprintf("ClusterRoleBinding/%s", id),
		}

		var output []string
		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(entry.Action, CreatedAction); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
			output = append(output, entry.Subject)
		}

		// verify the change set contains all objects in the right order
		if diff := cmp.Diff(expected, output); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	})

	t.Run("returns change set on failed apply", func(t *testing.T) {
		// update ClusterRoleBinding to trigger an immutable field error
		err = unstructured.SetNestedField(crb.Object, "test", "roleRef", "name")
		if err != nil {
			t.Fatal(err)
		}

		// apply and expect to fail
		changeSet, err := partialFailureManager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err == nil {
			t.Fatal("Expected error got none")
		}

		// expected change set after failed apply
		expected := []string{
			fmt.Sprintf("Namespace/%s", id),
			fmt.Sprintf("ClusterRole/%s", id),
			fmt.Sprintf("StorageClass/%s", id),
		}

		var output []string
		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(entry.Action, UnchangedAction); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
			output = append(output, entry.Subject)
		}

		// verify the change set contains all applied objects in the right order
		if diff := cmp.Diff(expected, output); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	})
}

func TestApply_Force(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("apply")
	objects, err := readManifest("testdata/test1.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")

	secretName, secret := getFirstObject(objects, "Secret", id)
	crbName, crb := getFirstObject(objects, "ClusterRoleBinding", id)
	stName, st := getFirstObject(objects, "StorageClass", id)
	_, svc := getFirstObject(objects, "Service", id)

	// create objects
	if _, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions()); err != nil {
		t.Fatal(err)
	}

	t.Run("fails to apply immutable secret", func(t *testing.T) {
		// update a value in the secret
		err = unstructured.SetNestedField(secret.Object, "val-secret", "stringData", "key")
		if err != nil {
			t.Fatal(err)
		}

		// apply and expect to fail
		_, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err == nil {
			t.Fatal("Expected error got none")
		}

		// verify that the error message does not contain sensitive information
		expectedErr := fmt.Sprintf(
			"%s dry-run failed (Invalid): Secret \"%s\" is invalid: data: Forbidden: field is immutable when `immutable` is set",
			utils.FmtUnstructured(secret), secret.GetName())
		if diff := cmp.Diff(expectedErr, err.Error()); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	})

	t.Run("force applies immutable secret", func(t *testing.T) {
		// force apply
		opts := DefaultApplyOptions()
		opts.Force = true

		changeSet, err := manager.ApplyAllStaged(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}

		// verify the secret was recreated
		for _, entry := range changeSet.Entries {
			if entry.Subject == secretName {
				if diff := cmp.Diff(CreatedAction, entry.Action); diff != "" {
					t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
				}
			} else {
				if diff := cmp.Diff(UnchangedAction, entry.Action); diff != "" {
					t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
				}
			}
		}

		// get the secret from cluster
		secretClone := secret.DeepCopy()
		err = manager.client.Get(ctx, client.ObjectKeyFromObject(secretClone), secretClone)
		if err != nil {
			t.Fatal(err)
		}

		// get data value from the in-cluster secret
		val, _, err := unstructured.NestedFieldCopy(secretClone.Object, "data", "key")
		if err != nil {
			t.Fatal(err)
		}

		// verify the secret was updated in cluster with the right data value
		if diff := cmp.Diff(val, base64.StdEncoding.EncodeToString([]byte("val-secret"))); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	})

	t.Run("force apply waits for finalizer", func(t *testing.T) {
		secretClone := secret.DeepCopy()
		{
			secretWithFinalizer := secretClone.DeepCopy()

			unstructured.SetNestedStringSlice(secretWithFinalizer.Object, []string{"fluxcd.io/demo-finalizer"}, "metadata", "finalizers")
			if err := manager.client.Update(ctx, secretWithFinalizer); err != nil {
				t.Fatal(err)
			}
		}

		// remove finalizer after a delay, to ensure the controller handles a slow deletion
		go func() {
			time.Sleep(3 * time.Second)

			secretWithoutFinalizer := secretClone.DeepCopy()
			unstructured.SetNestedStringSlice(secretWithoutFinalizer.Object, []string{}, "metadata", "finalizers")
			if err := manager.client.Update(ctx, secretWithoutFinalizer); err != nil {
				panic(err)
			}
		}()

		// update a value in the secret
		err = unstructured.SetNestedField(secret.Object, "val-secret2", "stringData", "key")
		if err != nil {
			t.Fatal(err)
		}

		// force apply
		opts := DefaultApplyOptions()
		opts.Force = true

		changeSet, err := manager.ApplyAllStaged(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}

		// verify the secret was recreated
		for _, entry := range changeSet.Entries {
			if entry.Subject == secretName {
				if diff := cmp.Diff(CreatedAction, entry.Action); diff != "" {
					t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
				}
			} else {
				if diff := cmp.Diff(UnchangedAction, entry.Action); diff != "" {
					t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
				}
			}
		}
	})

	t.Run("recreates immutable RBAC", func(t *testing.T) {
		// update roleRef
		err = unstructured.SetNestedField(crb.Object, "test", "roleRef", "name")
		if err != nil {
			t.Fatal(err)
		}

		// force apply
		opts := DefaultApplyOptions()
		opts.Force = true

		changeSet, err := manager.ApplyAllStaged(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}

		// verify the binding was recreated
		for _, entry := range changeSet.Entries {
			if entry.Subject == crbName {
				if diff := cmp.Diff(CreatedAction, entry.Action); diff != "" {
					t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
				}
				break
			}
		}
	})

	t.Run("recreates immutable StorageClass based on metadata", func(t *testing.T) {
		// update parameters
		err = unstructured.SetNestedField(st.Object, "true", "parameters", "encrypted")
		if err != nil {
			t.Fatal(err)
		}

		meta := map[string]string{
			"fluxcd.io/force": "true",
		}
		st.SetAnnotations(meta)

		// apply and expect to fail
		_, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err == nil {
			t.Fatal("Expected error got none")
		}

		// force apply selector
		opts := DefaultApplyOptions()
		opts.ForceSelector = meta

		changeSet, err := manager.ApplyAllStaged(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}

		// verify the storage class was recreated
		for _, entry := range changeSet.Entries {
			if entry.Subject == stName {
				if diff := cmp.Diff(CreatedAction, entry.Action); diff != "" {
					t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
				}
				break
			}
		}
	})

	t.Run("force apply returns validation error", func(t *testing.T) {
		// update to invalid yaml
		err = unstructured.SetNestedField(svc.Object, "ClusterIPSS", "spec", "type")
		if err != nil {
			t.Fatal(err)
		}

		// force apply objects
		opts := DefaultApplyOptions()
		opts.Force = true

		_, err := manager.ApplyAllStaged(ctx, objects, opts)
		if err == nil {
			t.Fatal("expected validation error but got none")
		}

		// should return validation error
		if !strings.Contains(err.Error(), "is invalid") {
			t.Errorf("expected error to contain invalid msg but got: %s", err)
		}
	})
}

func TestApply_SetNativeKindsDefaults(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("fix")
	objects, err := readManifest("testdata/test2.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")

	if err := normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	t.Run("creates objects", func(t *testing.T) {
		// create objects
		_, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}
	})

	// re-apply objects
	changeSet, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
	if err != nil {
		t.Fatal(err)
	}

	// verify that the change set contains no changed objects
	for _, entry := range changeSet.Entries {
		if diff := cmp.Diff(UnchangedAction, entry.Action); diff != "" {
			t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
		}
	}

}

func TestApply_NoOp(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("fix")
	objects, err := readManifest("testdata/test3.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")

	if err := normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	t.Run("creates objects", func(t *testing.T) {
		// create objects
		_, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("skips apply", func(t *testing.T) {
		// apply changes
		changeSet, err := manager.ApplyAll(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range changeSet.Entries {
			if entry.Action != UnchangedAction {
				t.Errorf("Diff found for %s", entry.String())
			}
		}
	})
}

func TestApply_SkipsExcluded(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("fix")
	err := manager.client.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: id,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	objects, err := readManifest("testdata/test13.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")

	if err := normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	opts := DefaultApplyOptions()
	opts.ExclusionSelector = map[string]string{
		"ssa.fluxcd.io/exclude": "true",
	}
	skippedSubject := fmt.Sprintf("Secret/%[1]s/data-%[1]s-excluded", id)

	t.Run("Apply", func(t *testing.T) {
		changeSetEntry, err := manager.Apply(ctx, objects[0], opts)
		if err != nil {
			t.Fatal(err)
		}

		if changeSetEntry.Subject != skippedSubject {
			t.Errorf("Expected %s, got %s", skippedSubject, changeSetEntry.Subject)
		}
	})

	t.Run("ApplyAll", func(t *testing.T) {
		changeSet, err := manager.ApplyAll(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}

		var found bool
		for _, entry := range changeSet.Entries {
			if entry.Action != SkippedAction {
				continue
			}
			found = true
			if entry.Subject != skippedSubject {
				t.Errorf("Expected %s, got %s", skippedSubject, entry.Subject)
			}
			break
		}
		if !found {
			t.Errorf("Expected to find skipped entry for %s", skippedSubject)
		}
	})
}

func TestApply_Exclusions(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("ignore")
	objects, err := readManifest("testdata/test1.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	_, configMap := getFirstObject(objects, "ConfigMap", id)

	t.Run("creates objects", func(t *testing.T) {
		// create objects
		_, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("skips apply", func(t *testing.T) {
		// mutate in-cluster object
		configMapClone := configMap.DeepCopy()
		err = manager.client.Get(ctx, client.ObjectKeyFromObject(configMapClone), configMapClone)
		if err != nil {
			t.Fatal(err)
		}

		meta := map[string]string{
			"fluxcd.io/ignore": "true",
		}
		configMapClone.SetAnnotations(meta)

		if err := unstructured.SetNestedField(configMapClone.Object, "val", "data", "key"); err != nil {
			t.Fatal(err)
		}

		if err := manager.client.Update(ctx, configMapClone); err != nil {
			t.Fatal(err)
		}

		opts := DefaultApplyOptions()
		opts.ExclusionSelector = meta

		// apply with exclusions
		changeSet, err := manager.ApplyAll(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range changeSet.Entries {
			if entry.Action == ConfiguredAction {
				t.Errorf("Diff found for %s", entry.String())
			}
		}
	})

	t.Run("applies changes", func(t *testing.T) {
		// apply changes without exclusions
		changeSet, err := manager.ApplyAll(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range changeSet.Entries {
			if entry.Action != ConfiguredAction && entry.Subject == utils.FmtUnstructured(configMap) {
				t.Errorf("Expected %s, got %s", ConfiguredAction, entry.Action)
			}
		}
	})

	t.Run("skips apply when desired state is annotated", func(t *testing.T) {
		configMapClone := configMap.DeepCopy()
		meta := map[string]string{
			"fluxcd.io/ignore": "true",
		}
		configMapClone.SetAnnotations(meta)

		// apply changes without exclusions
		changeSet, err := manager.Apply(ctx, configMapClone, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}

		if changeSet.Action != UnchangedAction {
			t.Errorf("Diff found for %s", changeSet.String())
		}
	})
}

func TestApply_IfNotPresent(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	meta := map[string]string{
		"fluxcd.io/ssa": "IfNotPresent",
	}

	id := generateName("skip")
	objects, err := readManifest("testdata/test1.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	_, ns := getFirstObject(objects, "Namespace", id)
	_, configMap := getFirstObject(objects, "ConfigMap", id)
	configMapClone := configMap.DeepCopy()
	configMapClone.SetAnnotations(meta)

	t.Run("creates objects", func(t *testing.T) {
		// create objects
		opts := DefaultApplyOptions()
		opts.IfNotPresentSelector = meta

		changeSet, err := manager.ApplyAllStaged(ctx, []*unstructured.Unstructured{ns, configMapClone}, opts)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range changeSet.Entries {
			if entry.Action != CreatedAction {
				t.Errorf("Expected %s, got %s for %s", CreatedAction, entry.Action, entry.Subject)
			}
		}
	})

	t.Run("skips apply when annotated IfNotPresent", func(t *testing.T) {
		opts := DefaultApplyOptions()
		opts.IfNotPresentSelector = meta

		changeSet, err := manager.Apply(ctx, configMapClone, opts)
		if err != nil {
			t.Fatal(err)
		}

		if changeSet.Action != SkippedAction {
			t.Errorf("Diff found for %s", changeSet.String())
		}
	})

	t.Run("resume apply when is annotated Override", func(t *testing.T) {
		override := map[string]string{
			"fluxcd.io/ssa": "Override",
		}
		configMapClone.SetAnnotations(override)

		opts := DefaultApplyOptions()
		opts.IfNotPresentSelector = meta

		changeSet, err := manager.Apply(ctx, configMapClone, opts)
		if err != nil {
			t.Fatal(err)
		}

		if changeSet.Action != ConfiguredAction {
			t.Errorf("Diff found for %s", changeSet.String())
		}
	})
}

func TestApply_Cleanup_ExactMatch(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("cleanup-exact")
	objects, err := readManifest("testdata/test2.yaml", id)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetOwnerLabels(objects, "app1", "default")

	_, deployObject := getFirstObject(objects, "Deployment", id)

	if err = normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	t.Run("creates objects as different managers", func(t *testing.T) {
		// Apply all with prefix manager
		for _, object := range objects {
			obj := object.DeepCopy()
			if err := manager.client.Patch(ctx, obj, client.Apply, client.FieldOwner("flux-apply-prefix")); err != nil {
				t.Fatal(err)
			}
		}

		// Apply deployment with exact match manager
		deploy := deployObject.DeepCopy()
		if err := manager.client.Patch(ctx, deploy, client.Apply, client.FieldOwner("flux")); err != nil {
			t.Fatal(err)
		}

		// Check that the deployment has both managers
		resultDeploy := deployObject.DeepCopy()
		err = manager.Client().Get(ctx, client.ObjectKeyFromObject(deploy), resultDeploy)
		if err != nil {
			t.Fatal(err)
		}

		managedFields := resultDeploy.GetManagedFields()
		foundExact := false
		foundPrefix := false

		for _, field := range managedFields {
			if field.Manager == "flux" && field.Operation == metav1.ManagedFieldsOperationApply {
				foundExact = true
			}
			if field.Manager == "flux-apply-prefix" && field.Operation == metav1.ManagedFieldsOperationApply {
				foundPrefix = true
			}
		}

		if !foundExact {
			t.Errorf("Expected to find exact match manager 'flux' with Apply operation")
		}
		if !foundPrefix {
			t.Errorf("Expected to find prefix manager 'flux-apply-prefix' with Apply operation")
		}
	})

	t.Run("cleanup removes only exact match", func(t *testing.T) {
		applyOpts := DefaultApplyOptions()
		applyOpts.Cleanup = ApplyCleanupOptions{
			FieldManagers: []FieldManager{
				{
					Name:          "flux",
					OperationType: metav1.ManagedFieldsOperationApply,
					ExactMatch:    true,
				},
			},
		}

		_, err := manager.ApplyAllStaged(ctx, objects, applyOpts)
		if err != nil {
			t.Fatal(err)
		}

		// Check that only exact match was removed
		resultDeploy := deployObject.DeepCopy()
		err = manager.Client().Get(ctx, client.ObjectKeyFromObject(resultDeploy), resultDeploy)
		if err != nil {
			t.Fatal(err)
		}

		managedFields := resultDeploy.GetManagedFields()
		foundExact := false
		foundPrefix := false
		foundManager := false

		for _, field := range managedFields {
			t.Logf("Found managed field: Manager=%s, Operation=%s", field.Manager, field.Operation)
			if field.Manager == "flux" {
				foundExact = true
			}
			if field.Manager == "flux-apply-prefix" {
				foundPrefix = true
			}
			if field.Manager == manager.owner.Field {
				foundManager = true
			}
		}

		if foundExact {
			t.Errorf("Expected exact match 'flux' to be removed, but it was still present")
		}
		if !foundPrefix {
			t.Errorf("Expected prefix match 'flux-apply-prefix' to remain, but it was not found")
		}
		if !foundManager {
			t.Errorf("Expected manager '%s' to be present, but it was not found", manager.owner.Field)
		}
	})
}

func TestApply_Cleanup(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	applyOpts := DefaultApplyOptions()
	applyOpts.Cleanup = ApplyCleanupOptions{
		Annotations: []string{corev1.LastAppliedConfigAnnotation},
		FieldManagers: []FieldManager{
			{
				Name:          "kubectl",
				OperationType: metav1.ManagedFieldsOperationApply,
			},
			{
				Name:          "kubectl",
				OperationType: metav1.ManagedFieldsOperationUpdate,
			},
			{
				Name:          "before-first-apply",
				OperationType: metav1.ManagedFieldsOperationUpdate,
			},
		},
	}

	id := generateName("cleanup")
	objects, err := readManifest("testdata/test2.yaml", id)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetOwnerLabels(objects, "app1", "default")

	_, deployObject := getFirstObject(objects, "Deployment", id)

	if err = normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	t.Run("creates objects as kubectl", func(t *testing.T) {
		for _, object := range objects {
			obj := object.DeepCopy()
			obj.SetAnnotations(map[string]string{corev1.LastAppliedConfigAnnotation: "test"})
			labels := obj.GetLabels()
			labels[corev1.LastAppliedConfigAnnotation] = "test"
			obj.SetLabels(labels)
			if err := manager.client.Create(ctx, obj, client.FieldOwner("kubectl-client-side-apply")); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("removes kubectl client-side-apply manager and annotation", func(t *testing.T) {
		applyOpts.Cleanup.Labels = []string{corev1.LastAppliedConfigAnnotation}
		changeSet, err := manager.ApplyAllStaged(ctx, objects, applyOpts)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(ConfiguredAction, entry.Action); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
		}

		deploy := deployObject.DeepCopy()
		err = manager.Client().Get(ctx, client.ObjectKeyFromObject(deploy), deploy)
		if err != nil {
			t.Fatal(err)
		}

		if _, ok := deploy.GetAnnotations()[corev1.LastAppliedConfigAnnotation]; ok {
			t.Errorf("%s annotation not removed", corev1.LastAppliedConfigAnnotation)
		}

		if _, ok := deploy.GetLabels()[corev1.LastAppliedConfigAnnotation]; ok {
			t.Errorf("%s label not removed", corev1.LastAppliedConfigAnnotation)
		}

		expectedManagers := []string{"before-first-apply", manager.owner.Field}
		for _, entry := range deploy.GetManagedFields() {
			if !containsItemString(expectedManagers, entry.Manager) {
				t.Log(entry)
				t.Errorf("Mismatch from expected values, want %v got %s", expectedManagers, entry.Manager)
			}
		}
	})

	t.Run("replaces kubectl server-side-apply manager", func(t *testing.T) {
		for _, object := range objects {
			obj := object.DeepCopy()
			if err := manager.client.Patch(ctx, obj, client.Apply, client.FieldOwner("kubectl")); err != nil {
				t.Fatal(err)
			}
		}

		deploy := deployObject.DeepCopy()
		err = manager.Client().Get(ctx, client.ObjectKeyFromObject(deploy), deploy)
		if err != nil {
			t.Fatal(err)
		}

		changeSet, err := manager.ApplyAll(ctx, objects, applyOpts)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(ConfiguredAction, entry.Action); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
		}

		deploy = deployObject.DeepCopy()
		err = manager.Client().Get(ctx, client.ObjectKeyFromObject(deploy), deploy)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range deploy.GetManagedFields() {
			if diff := cmp.Diff(manager.owner.Field, entry.Manager); diff != "" {
				t.Log(entry)
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
		}
	})
}

func TestApply_CleanupRemovals(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	applyOpts := DefaultApplyOptions()
	applyOpts.Cleanup = ApplyCleanupOptions{
		Annotations: []string{corev1.LastAppliedConfigAnnotation},
		FieldManagers: []FieldManager{
			{
				Name:          "kubectl",
				OperationType: metav1.ManagedFieldsOperationApply,
			},
			{
				Name:          "kubectl",
				OperationType: metav1.ManagedFieldsOperationUpdate,
			},
			{
				Name:          "before-first-apply",
				OperationType: metav1.ManagedFieldsOperationUpdate,
			},
		},
	}

	id := generateName("cleanup-removal")
	objects, err := readManifest("testdata/test8.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	editedObjects, err := readManifest("testdata/test9.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")
	manager.SetOwnerLabels(editedObjects, "app1", "default")

	_, ingressObject := getFirstObject(objects, "Ingress", id)

	t.Run("creates objects using manager apply", func(t *testing.T) {
		applyOpts.Cleanup.Labels = []string{corev1.LastAppliedConfigAnnotation}
		_, err := manager.ApplyAllStaged(ctx, objects, applyOpts)
		if err != nil {
			t.Fatal(err)
		}

		ingress := ingressObject.DeepCopy()
		err = manager.Client().Get(ctx, client.ObjectKeyFromObject(ingress), ingress)
		if err != nil {
			t.Fatal(err)
		}

		expectedManagers := []string{manager.owner.Field}
		for _, entry := range ingress.GetManagedFields() {
			if !containsItemString(expectedManagers, entry.Manager) {
				t.Log(entry)
				t.Errorf("Mismatch from expected values, want %v got %s", expectedManagers, entry.Manager)
			}
		}
	})

	t.Run("applies edited objects as kubectl", func(t *testing.T) {
		for _, object := range editedObjects {
			obj := object.DeepCopy()
			obj.SetAnnotations(map[string]string{corev1.LastAppliedConfigAnnotation: "test"})
			labels := obj.GetLabels()
			labels[corev1.LastAppliedConfigAnnotation] = "test"
			obj.SetLabels(labels)
			if err := manager.client.Patch(ctx, obj, client.Merge, client.FieldOwner("kubectl-client-side-apply")); err != nil {
				t.Fatal(err)
			}
		}

		ingress := ingressObject.DeepCopy()
		err = manager.Client().Get(ctx, client.ObjectKeyFromObject(ingress), ingress)
		if err != nil {
			t.Fatal(err)
		}

		expectedManagers := []string{"kubectl-client-side-apply", manager.owner.Field}
		for _, entry := range ingress.GetManagedFields() {
			if !containsItemString(expectedManagers, entry.Manager) {
				t.Log(entry)
				t.Errorf("Mismatch from expected values, want %v got %s", expectedManagers, entry.Manager)
			}
		}

		rules, _, err := unstructured.NestedSlice(ingress.Object, "spec", "rules")
		if err != nil {
			t.Fatal(err)
		}

		if len(rules) != 2 {
			t.Errorf("expected to two rules in Ingress, got %d", len(rules))
		}
	})

	t.Run("applies edited object using manager apply", func(t *testing.T) {
		applyOpts.Cleanup.Labels = []string{corev1.LastAppliedConfigAnnotation}
		_, err := manager.ApplyAllStaged(ctx, editedObjects, applyOpts)
		if err != nil {
			t.Fatal(err)
		}

		ingress := ingressObject.DeepCopy()
		err = manager.Client().Get(ctx, client.ObjectKeyFromObject(ingress), ingress)
		if err != nil {
			t.Fatal(err)
		}

		expectedManagers := []string{manager.owner.Field}
		for _, entry := range ingress.GetManagedFields() {
			if !containsItemString(expectedManagers, entry.Manager) {
				t.Log(entry)
				t.Errorf("Mismatch from expected values, want %v got %s", expectedManagers, entry.Manager)
			}
		}

		tlsMap, exists, err := unstructured.NestedSlice(ingress.Object, "spec", "tls")
		if err != nil {
			t.Fatalf("unexpected error while getting field from object: %s", err)
		}

		if exists {
			t.Errorf("spec.tls shouldn't be present, got %s", tlsMap)
		}
	})
}

func TestApply_Cleanup_Exclusions(t *testing.T) {
	kubectlManager := "kubectl-client-side-apply"
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	applyOpts := DefaultApplyOptions()
	applyOpts.Cleanup = ApplyCleanupOptions{
		Annotations: []string{corev1.LastAppliedConfigAnnotation},
		FieldManagers: []FieldManager{
			{
				Name:          "kubectl",
				OperationType: metav1.ManagedFieldsOperationApply,
			},
			{
				Name:          "kubectl",
				OperationType: metav1.ManagedFieldsOperationUpdate,
			},
			{
				Name:          "before-first-apply",
				OperationType: metav1.ManagedFieldsOperationUpdate,
			},
		},
		Exclusions: map[string]string{"cleanup/exclusion": "true"},
	}

	id := generateName("cleanup")
	objects, err := readManifest("testdata/test2.yaml", id)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetOwnerLabels(objects, "app1", "default")

	_, deployObject := getFirstObject(objects, "Deployment", id)

	if err = normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	t.Run("creates objects as kubectl", func(t *testing.T) {
		for _, object := range objects {
			obj := object.DeepCopy()
			if err := manager.client.Create(ctx, obj, client.FieldOwner(kubectlManager)); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("does not not remove kubectl manager", func(t *testing.T) {
		for _, object := range objects {
			object.SetAnnotations(map[string]string{"cleanup/exclusion": "true"})
		}

		changeSet, err := manager.ApplyAllStaged(ctx, objects, applyOpts)
		if err != nil {
			t.Fatal(err)
		}

		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(ConfiguredAction, entry.Action); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
		}

		deploy := deployObject.DeepCopy()
		err = manager.Client().Get(ctx, client.ObjectKeyFromObject(deploy), deploy)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, entry := range deploy.GetManagedFields() {
			if entry.Manager == kubectlManager {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Mismatch from expected values, want %v manager", kubectlManager)
		}
	})
}

func TestApply_MissingNamespaceErr(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("err")
	objects, err := readManifest("testdata/test1.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	_, configMap := getFirstObject(objects, "ConfigMap", id)
	unstructured.RemoveNestedField(configMap.Object, "metadata", "namespace")

	_, err = manager.ApplyAllStaged(ctx, []*unstructured.Unstructured{configMap}, DefaultApplyOptions())
	if !strings.Contains(err.Error(), "namespace not specified") {
		t.Fatal("Expected namespace not specified error")
	}
}

func containsItemString(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// exampleCert was generated from crypto/tls/generate_cert.go with the following command:
//
//	go run generate_cert.go  --rsa-bits 2048 --host example.com --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h - from
//
// this example is from https://github.com/kubernetes/kubernetes/blob/04d2f336419b5a824cb96cb88462ef18a90d619d/staging/src/k8s.io/apiserver/pkg/util/webhook/validation_test.go
// Base64 encoded because caBundle field expects base64 string when stored in unstructured.Unstructured
var exampleCert = base64.StdEncoding.EncodeToString([]byte(`-----BEGIN CERTIFICATE-----
MIIDIDCCAgigAwIBAgIRALYg7UBIx7aeUpwohjIBhUEwDQYJKoZIhvcNAQELBQAw
EjEQMA4GA1UEChMHQWNtZSBDbzAgFw03MDAxMDEwMDAwMDBaGA8yMDg0MDEyOTE2
MDAwMFowEjEQMA4GA1UEChMHQWNtZSBDbzCCASIwDQYJKoZIhvcNAQEBBQADggEP
ADCCAQoCggEBANJuxq11hL2nB6nygf5/q7JRkPZCYuXwkaqZm7Bk8e9+WzEy9/EW
QtRP92IuKB8XysLY7a/vh9WOcUMw9zBICP754pBIUjgt2KveEYABDSkrAVWIGIO9
IN6crS3OvHiMKyShCvqMMho9wxyTbtnl3lrlcxVyLCmMahnoSyIwWiQ3TMT81eKt
FGEYXa8XEIJJFRX6wxtCgw0PqQy/NLM+G1QvYyKLSLm2cKUGH1A9RfAlMzsICOOf
Rx+/zCAgAfXnjg0SUXfgOjc/Y8EdVyMmBfCWMfovbpwCwULxlEDHHsjVZy5azZjm
E2AYW94BSdRd745M7fudchS6+9rGJi9lc5kCAwEAAaNvMG0wDgYDVR0PAQH/BAQD
AgKkMBMGA1UdJQQMMAoGCCsGAQUFBwMBMA8GA1UdEwEB/wQFMAMBAf8wHQYDVR0O
BBYEFL/WGYyHD90dPKo8SswyPSydkwG/MBYGA1UdEQQPMA2CC2V4YW1wbGUuY29t
MA0GCSqGSIb3DQEBCwUAA4IBAQAS9qnl6mTF/HHRZSfQypxBj1lsDwYz99PsDAyw
hoXetTVmkejsPe9EcQ5eBRook6dFIevXN9bY5dxYSjWoSg/kdsihJ3FsJsmAQEtK
eM8ko9uvtZ+i0LUfg2l3kima1/oX0MCvnuePGgl7quyBhGUeg5tOudiX07hETWPW
Kt/FgMvfzK63pqcJpLj2+2pnmieV3ploJjw1sIAboR3W5LO/9XgRK3h1vr1BbplZ
dhv6TGB0Y1Zc9N64gh0A3xDOrBSllAWYw/XM6TodhvahFyE48fYSFBZVfZ3TZTfd
Bdcg8G2SMXDSZoMBltEIO7ogTjNAqNUJ8MWZFNZz6HnE8UJC
-----END CERTIFICATE-----`))

func TestResourceManager_ApplyAllStaged_CRDWebhookCABundle(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	t.Run("removes invalid CA bundle and applies successfully", func(t *testing.T) {
		id := generateName("remove-ca-bundle-invalid")
		objects, err := readManifest("testdata/test11.yaml", id)
		if err != nil {
			t.Fatal(err)
		}
		_, crd := getFirstObject(objects, "CustomResourceDefinition", "webhooks.example.com")
		uniqueGroup := fmt.Sprintf("%s.example.com", id)
		crdName := fmt.Sprintf("webhooks.%s", uniqueGroup)
		crd.SetName(crdName)
		err = unstructured.SetNestedField(crd.Object, uniqueGroup, "spec", "group")
		if err != nil {
			t.Fatal(err)
		}
		invalidCABundle := "invalid-cert-data"
		err = unstructured.SetNestedField(crd.Object, invalidCABundle, "spec", "conversion", "webhook", "clientConfig", "caBundle")
		if err != nil {
			t.Fatal(err)
		}
		manager.SetOwnerLabels(objects, "test", "default")
		changeSet, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range changeSet.Entries {
			if entry.Action != CreatedAction {
				t.Errorf("Expected %s, got %s for %s", CreatedAction, entry.Action, entry.Subject)
			}
		}
		crdClone := crd.DeepCopy()
		err = manager.client.Get(ctx, client.ObjectKeyFromObject(crdClone), crdClone)
		if err != nil {
			t.Fatal(err)
		}
		clusterCABundle, found, err := unstructured.NestedString(crdClone.Object, "spec", "conversion", "webhook", "clientConfig", "caBundle")
		if err != nil {
			t.Fatal(err)
		}
		if found && clusterCABundle != "" {
			t.Errorf("Expected invalid CA bundle to be removed, but found: %s", clusterCABundle)
		}
	})
	t.Run("removes valid CA bundle non base64 encoded and applies successfully", func(t *testing.T) {
		id := generateName("remove-ca-bundle-non-base64")
		objects, err := readManifest("testdata/test11.yaml", id)
		if err != nil {
			t.Fatal(err)
		}
		_, crd := getFirstObject(objects, "CustomResourceDefinition", "webhooks.example.com")
		uniqueGroup := fmt.Sprintf("%s.example.com", id)
		crdName := fmt.Sprintf("webhooks.%s", uniqueGroup)
		crd.SetName(crdName)
		err = unstructured.SetNestedField(crd.Object, uniqueGroup, "spec", "group")
		if err != nil {
			t.Fatal(err)
		}
		invalidCABundle, _ := base64.StdEncoding.DecodeString(exampleCert)
		err = unstructured.SetNestedField(crd.Object, string(invalidCABundle), "spec", "conversion", "webhook", "clientConfig", "caBundle")
		if err != nil {
			t.Fatal(err)
		}
		manager.SetOwnerLabels(objects, "test", "default")
		changeSet, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range changeSet.Entries {
			if entry.Action != CreatedAction {
				t.Errorf("Expected %s, got %s for %s", CreatedAction, entry.Action, entry.Subject)
			}
		}
		crdClone := crd.DeepCopy()
		err = manager.client.Get(ctx, client.ObjectKeyFromObject(crdClone), crdClone)
		if err != nil {
			t.Fatal(err)
		}
		clusterCABundle, found, err := unstructured.NestedString(crdClone.Object, "spec", "conversion", "webhook", "clientConfig", "caBundle")
		if err != nil {
			t.Fatal(err)
		}
		if found && clusterCABundle != "" {
			t.Errorf("Expected invalid CA bundle to be removed, but found: %s", clusterCABundle)
		}
	})
	t.Run("preserves valid CA bundle and applies successfully", func(t *testing.T) {
		id := generateName("remove-ca-bundle-valid")
		objects, err := readManifest("testdata/test11.yaml", id)
		if err != nil {
			t.Fatal(err)
		}
		_, crd := getFirstObject(objects, "CustomResourceDefinition", "webhooks.example.com")
		uniqueGroup := fmt.Sprintf("%s.example.com", id)
		crdName := fmt.Sprintf("webhooks.%s", uniqueGroup)
		crd.SetName(crdName)
		err = unstructured.SetNestedField(crd.Object, uniqueGroup, "spec", "group")
		if err != nil {
			t.Fatal(err)
		}
		err = unstructured.SetNestedField(crd.Object, exampleCert, "spec", "conversion", "webhook", "clientConfig", "caBundle")
		if err != nil {
			t.Fatal(err)
		}
		manager.SetOwnerLabels(objects, "test", "default")
		changeSet, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range changeSet.Entries {
			if entry.Action != CreatedAction {
				t.Errorf("Expected %s, got %s for %s", CreatedAction, entry.Action, entry.Subject)
			}
		}
		crdClone := crd.DeepCopy()
		err = manager.client.Get(ctx, client.ObjectKeyFromObject(crdClone), crdClone)
		if err != nil {
			t.Fatal(err)
		}
		clusterCABundle, found, err := unstructured.NestedString(crdClone.Object, "spec", "conversion", "webhook", "clientConfig", "caBundle")
		if err != nil {
			t.Fatal(err)
		}
		if !found || clusterCABundle != exampleCert {
			t.Errorf("Expected valid CA bundle to be preserved, got: %s", clusterCABundle)
		}
	})
}

func TestApplyAllStaged_AppliesRoleAndRoleBinding(t *testing.T) {
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("custom-stage")

	// Create a non-cluster-admin client to ensure dry-run checks are not bypassed.
	customStageNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: id,
		},
	}
	if err := manager.client.Create(ctx, customStageNS); err != nil {
		t.Fatal(err)
	}
	customStageSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: id,
		},
	}
	if err := manager.client.Create(ctx, customStageSA); err != nil {
		t.Fatal(err)
	}
	customStageCR := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: id,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"rbac.authorization.k8s.io"},
				Resources: []string{"roles", "rolebindings"},
				Verbs:     []string{"create", "update", "delete", "get", "list", "watch", "patch"},
			},
			// Grant the same permissions that the test Role will grant,
			// so RBAC escalation prevention allows creating the Role and
			// RoleBinding.
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "list"},
			},
		},
	}
	if err := manager.client.Create(ctx, customStageCR); err != nil {
		t.Fatal(err)
	}
	customStageCRB := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: id,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     customStageCR.Name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      customStageSA.Name,
			Namespace: customStageSA.Namespace,
		}},
	}
	if err := manager.client.Create(ctx, customStageCRB); err != nil {
		t.Fatal(err)
	}

	// Create a manager with the non-cluster-admin client
	customStageManager := *manager
	customStageCfg := rest.CopyConfig(cfg)
	customStageCfg.Impersonate = rest.ImpersonationConfig{
		UserName: fmt.Sprintf("system:serviceaccount:%s:%s", customStageSA.Namespace, customStageSA.Name),
	}
	customStageClient, err := client.New(customStageCfg, client.Options{
		Mapper: restMapper,
	})
	if err != nil {
		t.Fatal(err)
	}
	customStageManager.client = customStageClient

	// Create a Role and RoleBinding that references the Role.
	role := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]any{
				"name":      "role",
				"namespace": id,
			},
			"rules": []any{
				map[string]any{
					"apiGroups": []any{""},
					"resources": []any{"configmaps"},
					"verbs":     []any{"get", "list"},
				},
			},
		},
	}

	roleBinding := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]any{
				"name":      "role-binding",
				"namespace": id,
			},
			"roleRef": map[string]any{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "role",
			},
			"subjects": []any{
				map[string]any{
					"kind":      "ServiceAccount",
					"name":      "default",
					"namespace": id,
				},
			},
		},
	}

	objects := []*unstructured.Unstructured{roleBinding, role}

	t.Run("does not apply Role and RoleBinding together without custom stage", func(t *testing.T) {
		opts := DefaultApplyOptions()

		_, err := customStageManager.ApplyAllStaged(ctx, objects, opts)
		if err == nil {
			t.Fatal("Expected error when applying RoleBinding before Role, got none")
		}

		// Assert the error is a DryRunErr
		var dryRunErr *ssaerrors.DryRunErr
		if !errors.As(err, &dryRunErr) {
			t.Fatalf("Expected error to be *errors.DryRunErr, got %T", err)
		}

		// Assert the underlying error is NotFound
		if !apierrors.IsNotFound(dryRunErr.Unwrap()) {
			t.Errorf("Expected underlying error to be NotFound, got: %v", dryRunErr.Unwrap())
		}

		// Assert the NotFound is for the Role that the RoleBinding references
		var statusErr *apierrors.StatusError
		if !errors.As(dryRunErr.Unwrap(), &statusErr) {
			t.Fatalf("Expected underlying error to be *apierrors.StatusError, got %T", dryRunErr.Unwrap())
		}
		if statusErr.ErrStatus.Details == nil || statusErr.ErrStatus.Details.Name != "role" {
			t.Errorf("Expected NotFound to be for the Role named 'role', got: %+v", statusErr.ErrStatus.Details)
		}

		// Assert the involved object is the RoleBinding
		if dryRunErr.InvolvedObject().GetKind() != "RoleBinding" {
			t.Errorf("Expected involved object to be RoleBinding, got %s", dryRunErr.InvolvedObject().GetKind())
		}
	})

	t.Run("applies Role and RoleBinding together with Role in custom stage", func(t *testing.T) {
		opts := DefaultApplyOptions()
		opts.CustomStageKinds = map[schema.GroupKind]struct{}{
			{Group: "rbac.authorization.k8s.io", Kind: "Role"}: {},
		}

		changeSet, err := customStageManager.ApplyAllStaged(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}

		// Verify both objects were created
		if len(changeSet.Entries) != 2 {
			t.Errorf("Expected 2 entries, got %d", len(changeSet.Entries))
		}

		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(entry.Action, CreatedAction); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
		}
	})
}

func TestApply_DriftIgnoreRules_OptionalFields(t *testing.T) {
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("drift-opt")
	objects, err := readManifest("testdata/test2.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")

	if err := normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	_, deployObject := getFirstObject(objects, "Deployment", id)

	// Define ignore rules for two optional mutable fields upfront.
	opts := DefaultApplyOptions()
	opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
		{
			Paths: []string{"/spec/replicas"},
			Selector: &jsondiff.Selector{
				Kind: "Deployment",
			},
		},
		{
			Paths: []string{"/spec/template/metadata/annotations"},
			Selector: &jsondiff.Selector{
				Kind: "Deployment",
			},
		},
	}

	t.Run("creates objects with ignore rules present", func(t *testing.T) {
		// Set replicas so it's explicit in the desired state.
		err := unstructured.SetNestedField(deployObject.Object, int64(2), "spec", "replicas")
		if err != nil {
			t.Fatal(err)
		}

		changeSet, err := manager.ApplyAllStaged(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(CreatedAction, entry.Action); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
		}

		// On create, ignore rules are skipped, so Flux should own all fields
		// including replicas and annotations.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		replicas, found, _ := unstructured.NestedInt64(existing.Object, "spec", "replicas")
		if !found || replicas != 2 {
			t.Fatalf("expected spec.replicas=2 after create, got %d (found=%v)", replicas, found)
		}

		// Verify Flux is the field manager and owns both replicas and annotations.
		fluxFound := false
		for _, entry := range existing.GetManagedFields() {
			if entry.Manager == manager.owner.Field && entry.Operation == metav1.ManagedFieldsOperationApply {
				fluxFound = true
				if entry.FieldsV1 != nil {
					fieldsJSON := string(entry.FieldsV1.Raw)
					if !strings.Contains(fieldsJSON, "f:replicas") {
						t.Errorf("expected Flux to own spec.replicas after create, but it does not")
					}
					if !strings.Contains(fieldsJSON, "f:prometheus.io/scrape") {
						t.Errorf("expected Flux to own template annotations after create, but it does not")
					}
				}
			}
		}
		if !fluxFound {
			t.Errorf("expected to find field manager %q with Apply operation", manager.owner.Field)
		}
	})

	t.Run("other controllers claim ignored fields", func(t *testing.T) {
		// VPA controller claims spec.replicas via ForceOwnership.
		vpaObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      id,
					"namespace": id,
				},
				"spec": map[string]interface{}{
					"replicas": int64(5),
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": id,
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app": id,
							},
						},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "podinfod",
									"image": "ghcr.io/stefanprodan/podinfo:6.0.0",
								},
							},
						},
					},
				},
			},
		}
		err := manager.client.Patch(ctx, vpaObj, client.Apply,
			client.FieldOwner("vpa-controller"), client.ForceOwnership)
		if err != nil {
			t.Fatal(err)
		}

		// Monitoring controller claims template annotations via ForceOwnership.
		monObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      id,
					"namespace": id,
				},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": id,
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"annotations": map[string]interface{}{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "9797",
							},
							"labels": map[string]interface{}{
								"app": id,
							},
						},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "podinfod",
									"image": "ghcr.io/stefanprodan/podinfo:6.0.0",
								},
							},
						},
					},
				},
			},
		}
		err = manager.client.Patch(ctx, monObj, client.Apply,
			client.FieldOwner("monitoring-controller"), client.ForceOwnership)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the other controllers' values are in-cluster.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		replicas, _, _ := unstructured.NestedInt64(existing.Object, "spec", "replicas")
		if replicas != 5 {
			t.Fatalf("expected spec.replicas=5 after VPA claim, got %d", replicas)
		}

		// Verify field ownership transferred to the third-party controllers.
		vpaOwnsReplicas := false
		monOwnsAnnotations := false
		for _, entry := range existing.GetManagedFields() {
			if entry.Manager == "vpa-controller" && entry.Operation == metav1.ManagedFieldsOperationApply {
				if entry.FieldsV1 != nil {
					fieldsJSON := string(entry.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "f:replicas") {
						vpaOwnsReplicas = true
					}
				}
			}
			if entry.Manager == "monitoring-controller" && entry.Operation == metav1.ManagedFieldsOperationApply {
				if entry.FieldsV1 != nil {
					fieldsJSON := string(entry.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "f:prometheus.io/scrape") {
						monOwnsAnnotations = true
					}
				}
			}
		}
		if !vpaOwnsReplicas {
			t.Errorf("expected vpa-controller to own spec.replicas after ForceOwnership claim")
		}
		if !monOwnsAnnotations {
			t.Errorf("expected monitoring-controller to own template annotations after ForceOwnership claim")
		}
	})

	t.Run("flux apply releases ownership of ignored fields", func(t *testing.T) {
		// Trigger drift by changing a non-ignored field.
		err := unstructured.SetNestedField(deployObject.Object, int64(10), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}

		entry, err := manager.Apply(ctx, deployObject, opts)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Action != ConfiguredAction {
			t.Errorf("expected ConfiguredAction, got %s", entry.Action)
		}

		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}

		// VPA's replicas value should be preserved.
		replicas, found, _ := unstructured.NestedInt64(existing.Object, "spec", "replicas")
		if !found || replicas != 5 {
			t.Errorf("expected spec.replicas=5 (VPA value preserved), got %d", replicas)
		}

		// Verify Flux no longer owns the ignored fields.
		for _, mf := range existing.GetManagedFields() {
			if mf.Manager == manager.owner.Field && mf.Operation == metav1.ManagedFieldsOperationApply {
				if mf.FieldsV1 != nil {
					fieldsJSON := string(mf.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "f:replicas") {
						t.Errorf("expected Flux to no longer own spec.replicas")
					}
					if strings.Contains(fieldsJSON, "f:prometheus.io/scrape") {
						t.Errorf("expected Flux to no longer own prometheus.io/scrape")
					}
					if strings.Contains(fieldsJSON, "f:prometheus.io/port") {
						t.Errorf("expected Flux to no longer own prometheus.io/port")
					}
				}
			}
		}
	})

	t.Run("other controller orphans ignored field and flux does not reclaim it", func(t *testing.T) {
		// VPA applies WITHOUT spec.replicas, dropping its ownership.
		// Since Flux also doesn't own it anymore (released in previous subtest),
		// the field becomes orphaned — no manager owns it.
		vpaObjNoReplicas := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      id,
					"namespace": id,
				},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": id,
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app": id,
							},
						},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "podinfod",
									"image": "ghcr.io/stefanprodan/podinfo:6.0.0",
								},
							},
						},
					},
				},
			},
		}
		err := manager.client.Patch(ctx, vpaObjNoReplicas, client.Apply,
			client.FieldOwner("vpa-controller"))
		if err != nil {
			t.Fatal(err)
		}

		// Verify replicas is still in-cluster but orphaned (no manager owns it).
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		replicas, found, _ := unstructured.NestedInt64(existing.Object, "spec", "replicas")
		if !found {
			t.Fatal("expected spec.replicas to still exist in-cluster after VPA dropped ownership")
		}
		t.Logf("spec.replicas=%d is now orphaned (value persists, no manager owns it)", replicas)

		// Confirm no manager owns spec.replicas.
		for _, mf := range existing.GetManagedFields() {
			if mf.FieldsV1 != nil {
				fieldsJSON := string(mf.FieldsV1.Raw)
				if strings.Contains(fieldsJSON, "f:replicas") {
					t.Errorf("expected no manager to own spec.replicas, but %q (op=%s) still owns it",
						mf.Manager, mf.Operation)
				}
			}
		}

		// Flux re-apply with ignore rule should NOT reclaim the orphaned field.
		err = unstructured.SetNestedField(deployObject.Object, int64(11), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}

		entry, err := manager.Apply(ctx, deployObject, opts)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Action != ConfiguredAction {
			t.Errorf("expected ConfiguredAction, got %s", entry.Action)
		}

		// Verify Flux did NOT reclaim ownership of spec.replicas.
		existing = deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		for _, mf := range existing.GetManagedFields() {
			if mf.FieldsV1 != nil {
				fieldsJSON := string(mf.FieldsV1.Raw)
				if strings.Contains(fieldsJSON, "f:replicas") {
					t.Errorf("expected spec.replicas to remain orphaned, but %q (op=%s) owns it",
						mf.Manager, mf.Operation)
				}
			}
		}
	})
}

func TestApply_DriftIgnoreRules_ImmutableField(t *testing.T) {
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("drift-imm")
	objects, err := readManifest("testdata/test2.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")

	if err := normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	_, deployObject := getFirstObject(objects, "Deployment", id)

	// Define ignore rule for the immutable spec.selector field upfront.
	opts := DefaultApplyOptions()
	opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
		{
			Paths: []string{"/spec/selector"},
			Selector: &jsondiff.Selector{
				Kind: "Deployment",
			},
		},
	}

	t.Run("creates objects with ignore rules present", func(t *testing.T) {
		changeSet, err := manager.ApplyAllStaged(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(CreatedAction, entry.Action); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
		}
	})

	t.Run("flux apply fails when sole owner ignores immutable field", func(t *testing.T) {
		// When Flux is the sole owner of spec.selector and the ignore rule strips
		// it from the payload, K8s tries to remove the field. Since it's immutable
		// and required, the apply is rejected.
		err := unstructured.SetNestedField(deployObject.Object, int64(7), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}

		_, applyErr := manager.Apply(ctx, deployObject, opts)
		if applyErr == nil {
			t.Fatal("expected apply to fail when Flux is sole owner and ignores immutable field spec.selector")
		}
		t.Logf("Apply correctly failed when sole owner ignores immutable field: %v", applyErr)

		if !strings.Contains(applyErr.Error(), "spec.selector") {
			t.Errorf("expected error to mention spec.selector, got: %v", applyErr)
		}

		// Verify the Deployment is still intact in-cluster.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		_, found, _ := unstructured.NestedMap(existing.Object, "spec", "selector")
		if !found {
			t.Fatal("expected spec.selector to still exist in-cluster after failed apply")
		}
	})

	t.Run("other controller co-owns immutable field", func(t *testing.T) {
		// Another controller applies with the same selector value. Since the value
		// matches, both Flux and selector-controller co-own spec.selector.
		otherObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      id,
					"namespace": id,
				},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": id,
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app": id,
							},
						},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "podinfod",
									"image": "ghcr.io/stefanprodan/podinfo:6.0.0",
								},
							},
						},
					},
				},
			},
		}
		err := manager.client.Patch(ctx, otherObj, client.Apply,
			client.FieldOwner("selector-controller"))
		if err != nil {
			t.Fatal(err)
		}

		// Verify both Flux and selector-controller co-own spec.selector.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		selectorControllerOwns := false
		fluxOwnsSelector := false
		for _, entry := range existing.GetManagedFields() {
			if entry.FieldsV1 != nil {
				fieldsJSON := string(entry.FieldsV1.Raw)
				if strings.Contains(fieldsJSON, "f:selector") {
					if entry.Manager == "selector-controller" && entry.Operation == metav1.ManagedFieldsOperationApply {
						selectorControllerOwns = true
					}
					if entry.Manager == manager.owner.Field && entry.Operation == metav1.ManagedFieldsOperationApply {
						fluxOwnsSelector = true
					}
				}
			}
		}
		if !selectorControllerOwns {
			t.Errorf("expected selector-controller to co-own spec.selector")
		}
		if !fluxOwnsSelector {
			t.Errorf("expected Flux to still co-own spec.selector before ignore-rule apply")
		}
	})

	t.Run("flux apply succeeds when co-owned immutable field is ignored", func(t *testing.T) {
		// Now that selector-controller co-owns spec.selector, Flux can safely
		// drop the field from its payload. K8s doesn't try to remove the field
		// because selector-controller still owns it. Flux just releases its
		// co-ownership.
		err := unstructured.SetNestedField(deployObject.Object, int64(8), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}

		entry, err := manager.Apply(ctx, deployObject, opts)
		if err != nil {
			t.Fatalf("expected apply to succeed when ignoring co-owned immutable field, got: %v", err)
		}

		if entry.Action != ConfiguredAction {
			t.Errorf("expected ConfiguredAction, got %s", entry.Action)
		}

		// Verify the Deployment is intact and spec.selector is preserved.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		_, found, _ := unstructured.NestedMap(existing.Object, "spec", "selector")
		if !found {
			t.Fatal("expected spec.selector to still exist in-cluster after apply")
		}

		// Verify Flux no longer owns spec.selector.
		fluxOwnsSelector := false
		selectorControllerOwns := false
		for _, mf := range existing.GetManagedFields() {
			if mf.FieldsV1 != nil {
				fieldsJSON := string(mf.FieldsV1.Raw)
				if strings.Contains(fieldsJSON, "f:selector") {
					if mf.Manager == manager.owner.Field && mf.Operation == metav1.ManagedFieldsOperationApply {
						fluxOwnsSelector = true
					}
					if mf.Manager == "selector-controller" && mf.Operation == metav1.ManagedFieldsOperationApply {
						selectorControllerOwns = true
					}
				}
			}
		}
		if fluxOwnsSelector {
			t.Errorf("expected Flux to no longer own spec.selector after ignore-rule apply")
		}
		if !selectorControllerOwns {
			t.Errorf("expected selector-controller to still own spec.selector")
		}
	})

	t.Run("immutable required field cannot be orphaned by other controller", func(t *testing.T) {
		// spec.selector is both immutable and required on Deployments.
		// The API server rejects an apply payload that omits it.
		// Verify that selector-controller cannot drop spec.selector from its payload.
		selectorObjNoSelector := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      id,
					"namespace": id,
				},
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app": id,
							},
						},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "podinfod",
									"image": "ghcr.io/stefanprodan/podinfo:6.0.0",
								},
							},
						},
					},
				},
			},
		}
		err := manager.client.Patch(ctx, selectorObjNoSelector, client.Apply,
			client.FieldOwner("selector-controller"))
		if err == nil {
			t.Fatal("expected API server to reject apply without required spec.selector field, but got no error")
		}
		if !strings.Contains(err.Error(), "field is immutable") {
			t.Errorf("expected error to mention field is immutable, got: %v", err)
		}

		// Verify the Deployment is still intact and spec.selector is preserved.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		_, found, _ := unstructured.NestedMap(existing.Object, "spec", "selector")
		if !found {
			t.Fatal("expected spec.selector to still exist in-cluster after rejected apply")
		}

		// Verify selector-controller still owns spec.selector after the rejected apply.
		selectorControllerOwns := false
		for _, mf := range existing.GetManagedFields() {
			if mf.Manager == "selector-controller" && mf.Operation == metav1.ManagedFieldsOperationApply {
				if mf.FieldsV1 != nil {
					fieldsJSON := string(mf.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "f:selector") {
						selectorControllerOwns = true
					}
				}
			}
		}
		if !selectorControllerOwns {
			t.Errorf("expected selector-controller to still own spec.selector after rejected apply")
		}
	})
}

func TestApply_DriftIgnoreRules_RequiredMutableField(t *testing.T) {
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("drift-mut")
	objects, err := readManifest("testdata/test2.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")

	if err := normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	_, deployObject := getFirstObject(objects, "Deployment", id)

	// Define ignore rule for the container image (required but mutable) upfront.
	// The path targets the image field of the first container.
	opts := DefaultApplyOptions()
	opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
		{
			Paths: []string{"/spec/template/spec/containers/0/image"},
			Selector: &jsondiff.Selector{
				Kind: "Deployment",
			},
		},
	}

	t.Run("creates objects with ignore rules present", func(t *testing.T) {
		changeSet, err := manager.ApplyAllStaged(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(CreatedAction, entry.Action); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
		}

		// On create, ignore rules are skipped, so the image should be present.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		containers, found, _ := unstructured.NestedSlice(existing.Object, "spec", "template", "spec", "containers")
		if !found || len(containers) == 0 {
			t.Fatal("expected containers to exist after create")
		}
		c0 := containers[0].(map[string]interface{})
		if c0["image"] != "ghcr.io/stefanprodan/podinfo:6.0.0" {
			t.Fatalf("expected image 6.0.0 after create, got %v", c0["image"])
		}
	})

	t.Run("image policy controller claims container image", func(t *testing.T) {
		// An image policy controller updates the container image and takes ownership.
		imgObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      id,
					"namespace": id,
				},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": id,
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app": id,
							},
						},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "podinfod",
									"image": "ghcr.io/stefanprodan/podinfo:6.2.0",
								},
							},
						},
					},
				},
			},
		}
		err := manager.client.Patch(ctx, imgObj, client.Apply,
			client.FieldOwner("image-policy-controller"), client.ForceOwnership)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the image was updated.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		containers, _, _ := unstructured.NestedSlice(existing.Object, "spec", "template", "spec", "containers")
		c0 := containers[0].(map[string]interface{})
		if c0["image"] != "ghcr.io/stefanprodan/podinfo:6.2.0" {
			t.Fatalf("expected image 6.2.0 after image-policy claim, got %v", c0["image"])
		}

		// Verify image-policy-controller owns the image field and Flux does not.
		imgControllerOwnsImage := false
		fluxOwnsImage := false
		for _, entry := range existing.GetManagedFields() {
			if entry.FieldsV1 != nil {
				fieldsJSON := string(entry.FieldsV1.Raw)
				if strings.Contains(fieldsJSON, "\"f:image\":") {
					if entry.Manager == "image-policy-controller" && entry.Operation == metav1.ManagedFieldsOperationApply {
						imgControllerOwnsImage = true
					}
					if entry.Manager == manager.owner.Field && entry.Operation == metav1.ManagedFieldsOperationApply {
						fluxOwnsImage = true
					}
				}
			}
		}
		if !imgControllerOwnsImage {
			t.Errorf("expected image-policy-controller to own container image after ForceOwnership claim")
		}
		if fluxOwnsImage {
			t.Errorf("expected Flux to no longer own container image after ForceOwnership takeover, but it still does")
		}
	})

	t.Run("flux apply releases image ownership and preserves other controller value", func(t *testing.T) {
		// Trigger drift by changing a non-ignored field.
		err := unstructured.SetNestedField(deployObject.Object, int64(12), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}

		entry, err := manager.Apply(ctx, deployObject, opts)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Action != ConfiguredAction {
			t.Errorf("expected ConfiguredAction, got %s", entry.Action)
		}

		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}

		// The image-policy-controller's value should be preserved.
		containers, found, _ := unstructured.NestedSlice(existing.Object, "spec", "template", "spec", "containers")
		if !found || len(containers) == 0 {
			t.Fatal("expected containers to still exist")
		}
		c0 := containers[0].(map[string]interface{})
		if c0["image"] != "ghcr.io/stefanprodan/podinfo:6.2.0" {
			t.Errorf("expected image-policy-controller's image 6.2.0 to be preserved, got %v", c0["image"])
		}

		// Verify Flux no longer owns the container image field.
		for _, mf := range existing.GetManagedFields() {
			if mf.Manager == manager.owner.Field && mf.Operation == metav1.ManagedFieldsOperationApply {
				if mf.FieldsV1 != nil {
					fieldsJSON := string(mf.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "\"f:image\":") {
						t.Errorf("expected Flux to no longer own container image, but it does")
					}
				}
			}
		}

		// Verify image-policy-controller still owns the image.
		imgControllerOwnsImage := false
		for _, mf := range existing.GetManagedFields() {
			if mf.Manager == "image-policy-controller" && mf.Operation == metav1.ManagedFieldsOperationApply {
				if mf.FieldsV1 != nil {
					fieldsJSON := string(mf.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "\"f:image\":") {
						imgControllerOwnsImage = true
					}
				}
			}
		}
		if !imgControllerOwnsImage {
			t.Errorf("expected image-policy-controller to still own container image")
		}
	})

	t.Run("required field cannot be orphaned by other controller", func(t *testing.T) {
		// Unlike optional fields like spec.replicas, the container image is a
		// required field. The API server rejects an apply payload that omits it.
		// Verify that image-policy-controller cannot drop the image field.
		imgObjNoImage := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      id,
					"namespace": id,
				},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app": id,
						},
					},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app": id,
							},
						},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"name": "podinfod",
								},
							},
						},
					},
				},
			},
		}
		err := manager.client.Patch(ctx, imgObjNoImage, client.Apply,
			client.FieldOwner("image-policy-controller"))
		if err == nil {
			t.Fatal("expected API server to reject apply without required image field, but got no error")
		}
		if !strings.Contains(err.Error(), "Required") {
			t.Errorf("expected error to mention required field, got: %v", err)
		}

		// Verify the Deployment is still intact and image-policy-controller still owns the image.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		containers, found, _ := unstructured.NestedSlice(existing.Object, "spec", "template", "spec", "containers")
		if !found || len(containers) == 0 {
			t.Fatal("expected containers to still exist after rejected apply")
		}
		c0 := containers[0].(map[string]interface{})
		if c0["image"] != "ghcr.io/stefanprodan/podinfo:6.2.0" {
			t.Errorf("expected image 6.2.0 to be preserved after rejected apply, got %v", c0["image"])
		}

		imgControllerOwnsImage := false
		for _, mf := range existing.GetManagedFields() {
			if mf.Manager == "image-policy-controller" && mf.Operation == metav1.ManagedFieldsOperationApply {
				if mf.FieldsV1 != nil {
					fieldsJSON := string(mf.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "\"f:image\":") {
						imgControllerOwnsImage = true
					}
				}
			}
		}
		if !imgControllerOwnsImage {
			t.Errorf("expected image-policy-controller to still own container image after rejected apply")
		}
	})
}

func TestApply_DriftIgnoreRules_SelectorAndPaths(t *testing.T) {
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("drift-sel")
	objects, err := readManifest("testdata/test2.yaml", id)
	if err != nil {
		t.Fatal(err)
	}

	manager.SetOwnerLabels(objects, "app1", "default")

	if err := normalize.UnstructuredList(objects); err != nil {
		t.Fatal(err)
	}

	_, deployObject := getFirstObject(objects, "Deployment", id)
	_, svcObject := getFirstObject(objects, "Service", id)

	t.Run("creates objects initially", func(t *testing.T) {
		changeSet, err := manager.ApplyAllStaged(ctx, objects, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range changeSet.Entries {
			if diff := cmp.Diff(CreatedAction, entry.Action); diff != "" {
				t.Errorf("Mismatch from expected value (-want +got):\n%s", diff)
			}
		}
	})

	t.Run("non-matching selector does not strip field", func(t *testing.T) {
		// An ignore rule targeting kind=Service should NOT affect a Deployment.
		// The Deployment's spec.minReadySeconds should be applied normally.
		opts := DefaultApplyOptions()
		opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
			{
				Paths: []string{"/spec/minReadySeconds"},
				Selector: &jsondiff.Selector{
					Kind: "Service",
				},
			},
		}

		err := unstructured.SetNestedField(deployObject.Object, int64(20), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}

		entry, err := manager.Apply(ctx, deployObject, opts)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Action != ConfiguredAction {
			t.Errorf("expected ConfiguredAction, got %s", entry.Action)
		}

		// Verify the field was applied (not ignored).
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		val, found, _ := unstructured.NestedInt64(existing.Object, "spec", "minReadySeconds")
		if !found || val != 20 {
			t.Errorf("expected spec.minReadySeconds=20 (not ignored), got %d (found=%v)", val, found)
		}
	})

	t.Run("name selector matches only named object", func(t *testing.T) {
		// An ignore rule targeting kind=Deployment, name=nonexistent should
		// NOT match the actual Deployment. The field should be applied normally.
		opts := DefaultApplyOptions()
		opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
			{
				Paths: []string{"/spec/minReadySeconds"},
				Selector: &jsondiff.Selector{
					Kind: "Deployment",
					Name: "nonexistent",
				},
			},
		}

		err := unstructured.SetNestedField(deployObject.Object, int64(22), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}

		entry, err := manager.Apply(ctx, deployObject, opts)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Action != ConfiguredAction {
			t.Errorf("expected ConfiguredAction, got %s", entry.Action)
		}

		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		val, _, _ := unstructured.NestedInt64(existing.Object, "spec", "minReadySeconds")
		if val != 22 {
			t.Errorf("expected spec.minReadySeconds=22 (selector didn't match), got %d", val)
		}
	})

	t.Run("multiple paths in single rule", func(t *testing.T) {
		// A single IgnoreRule with multiple Paths strips all listed fields.
		err := unstructured.SetNestedField(deployObject.Object, int64(3), "spec", "replicas")
		if err != nil {
			t.Fatal(err)
		}

		// First apply to claim ownership of replicas.
		_, err = manager.Apply(ctx, deployObject, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}

		opts := DefaultApplyOptions()
		opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
			{
				Paths: []string{"/spec/replicas", "/spec/template/metadata/annotations"},
				Selector: &jsondiff.Selector{
					Kind: "Deployment",
				},
			},
		}

		// Trigger drift.
		err = unstructured.SetNestedField(deployObject.Object, int64(25), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}

		entry, err := manager.Apply(ctx, deployObject, opts)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Action != ConfiguredAction {
			t.Errorf("expected ConfiguredAction, got %s", entry.Action)
		}

		// Verify Flux no longer owns either field.
		existing := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		for _, mf := range existing.GetManagedFields() {
			if mf.Manager == manager.owner.Field && mf.Operation == metav1.ManagedFieldsOperationApply {
				if mf.FieldsV1 != nil {
					fieldsJSON := string(mf.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "f:replicas") {
						t.Errorf("expected Flux to not own spec.replicas")
					}
					if strings.Contains(fieldsJSON, "f:prometheus.io/scrape") {
						t.Errorf("expected Flux to not own prometheus annotation")
					}
				}
			}
		}
	})

	t.Run("non-existent path is a no-op", func(t *testing.T) {
		// Ignoring a path that doesn't exist in the object should not error.
		opts := DefaultApplyOptions()
		opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
			{
				Paths: []string{"/spec/nonExistentField"},
				Selector: &jsondiff.Selector{
					Kind: "Deployment",
				},
			},
		}

		err := unstructured.SetNestedField(deployObject.Object, int64(27), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}

		entry, err := manager.Apply(ctx, deployObject, opts)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Action != ConfiguredAction {
			t.Errorf("expected ConfiguredAction, got %s", entry.Action)
		}
	})

	t.Run("ApplyAll applies ignore rules selectively", func(t *testing.T) {
		// An ignore rule targeting kind=Deployment should strip fields from the
		// Deployment but NOT from the Service. We verify by checking that the
		// Service's ports are still owned by Flux (not stripped).
		err := unstructured.SetNestedField(deployObject.Object, int64(3), "spec", "replicas")
		if err != nil {
			t.Fatal(err)
		}
		_, err = manager.Apply(ctx, deployObject, DefaultApplyOptions())
		if err != nil {
			t.Fatal(err)
		}

		opts := DefaultApplyOptions()
		opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
			{
				Paths: []string{"/spec/replicas"},
				Selector: &jsondiff.Selector{
					Kind: "Deployment",
				},
			},
		}

		// Trigger drift on both the Deployment and the Service.
		err = unstructured.SetNestedField(deployObject.Object, int64(30), "spec", "minReadySeconds")
		if err != nil {
			t.Fatal(err)
		}
		err = unstructured.SetNestedField(svcObject.Object, "ClientIP", "spec", "sessionAffinity")
		if err != nil {
			t.Fatal(err)
		}

		changeSet, err := manager.ApplyAll(ctx, objects, opts)
		if err != nil {
			t.Fatal(err)
		}

		// Verify we got a change set back.
		if len(changeSet.Entries) == 0 {
			t.Fatal("expected non-empty change set from ApplyAll")
		}

		// Verify Flux no longer owns Deployment's spec.replicas.
		existingDeploy := deployObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existingDeploy), existingDeploy); err != nil {
			t.Fatal(err)
		}
		for _, mf := range existingDeploy.GetManagedFields() {
			if mf.Manager == manager.owner.Field && mf.Operation == metav1.ManagedFieldsOperationApply {
				if mf.FieldsV1 != nil {
					fieldsJSON := string(mf.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "f:replicas") {
						t.Errorf("expected Flux to not own Deployment spec.replicas after ApplyAll")
					}
				}
			}
		}

		// Verify Service was NOT affected by the Deployment-targeted rule.
		existingSvc := svcObject.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existingSvc), existingSvc); err != nil {
			t.Fatal(err)
		}
		svcOwnedByFlux := false
		for _, mf := range existingSvc.GetManagedFields() {
			if mf.Manager == manager.owner.Field && mf.Operation == metav1.ManagedFieldsOperationApply {
				svcOwnedByFlux = true
			}
		}
		if !svcOwnedByFlux {
			t.Errorf("expected Flux to still own the Service (rule shouldn't affect it)")
		}
	})
}

func TestApply_DriftIgnoreRules_CreateSkipsIgnore(t *testing.T) {
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	id := generateName("drift-create")

	// Build a minimal ConfigMap inline to test create behavior in isolation.
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": id,
			},
		},
	}
	if _, err := manager.Apply(ctx, ns, DefaultApplyOptions()); err != nil {
		t.Fatal(err)
	}

	cm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      id,
				"namespace": id,
			},
			"data": map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	t.Run("create includes all fields despite ignore rules", func(t *testing.T) {
		// When creating a new object, ignore rules should NOT strip fields
		// because the guard `existingObject.GetResourceVersion() != ""` is false.
		opts := DefaultApplyOptions()
		opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
			{
				Paths: []string{"/data/key1"},
				Selector: &jsondiff.Selector{
					Kind: "ConfigMap",
				},
			},
		}

		entry, err := manager.Apply(ctx, cm, opts)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Action != CreatedAction {
			t.Errorf("expected CreatedAction, got %s", entry.Action)
		}

		// Verify key1 IS present in-cluster (not stripped on create).
		existing := cm.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		val, found, _ := unstructured.NestedString(existing.Object, "data", "key1")
		if !found || val != "value1" {
			t.Errorf("expected data.key1=value1 on create (ignore rules should not apply), got %q (found=%v)", val, found)
		}
	})

	t.Run("subsequent update strips ignored field", func(t *testing.T) {
		// On update, the ignore rule should now take effect.
		opts := DefaultApplyOptions()
		opts.DriftIgnoreRules = []jsondiff.IgnoreRule{
			{
				Paths: []string{"/data/key1"},
				Selector: &jsondiff.Selector{
					Kind: "ConfigMap",
				},
			},
		}

		// Change a non-ignored field to trigger drift.
		err := unstructured.SetNestedField(cm.Object, "value2-updated", "data", "key2")
		if err != nil {
			t.Fatal(err)
		}

		entry, err := manager.Apply(ctx, cm, opts)
		if err != nil {
			t.Fatal(err)
		}

		if entry.Action != ConfiguredAction {
			t.Errorf("expected ConfiguredAction, got %s", entry.Action)
		}

		// Verify Flux no longer owns data.key1.
		existing := cm.DeepCopy()
		if err := manager.client.Get(ctx, client.ObjectKeyFromObject(existing), existing); err != nil {
			t.Fatal(err)
		}
		for _, mf := range existing.GetManagedFields() {
			if mf.Manager == manager.owner.Field && mf.Operation == metav1.ManagedFieldsOperationApply {
				if mf.FieldsV1 != nil {
					fieldsJSON := string(mf.FieldsV1.Raw)
					if strings.Contains(fieldsJSON, "f:key1") {
						t.Errorf("expected Flux to not own data.key1 after update with ignore rule")
					}
				}
			}
		}
	})
}
