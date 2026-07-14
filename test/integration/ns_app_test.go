//go:build integration_tests

package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/bpfman/bpfman-operator/apis/v1alpha1"
)

const (
	nsSameNameAppName  = "app-test-namespaced"
	nsSameNameNs1      = "same-name-app-a"
	nsSameNameNs2      = "same-name-app-b"
	nsSameNameBytecode = "quay.io/bpfman-bytecode/go-app-counter:latest"
)

// TestNamespacedAppSameNameDifferentNamespaces checks that two
// BpfApplications sharing a name across different namespaces, with
// different program lists, each get their own BpfApplicationState
// objects and reconcile to Success independently. Previously the
// agent's state lookup was not scoped by namespace, so the second
// application adopted the first application's state object, drove it
// to Error, and never created its own, leaving the second application
// Pending forever.
//
// See https://redhat.atlassian.net/browse/BPFMAN-45.
func TestNamespacedAppSameNameDifferentNamespaces(t *testing.T) {
	// The pod selector deliberately matches no pods: the programs
	// load and the applications reconcile to Success with zero
	// attachments, which is all the state-object accounting this
	// test verifies needs.
	netNsSelector := v1alpha1.NetworkNamespaceSelector{
		Pods: metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "same-name-app-target",
			},
		},
	}

	tcProgram := v1alpha1.BpfApplicationProgram{
		Name: "stats",
		Type: v1alpha1.ProgTypeTC,
		TC: &v1alpha1.TcProgramInfo{
			Links: []v1alpha1.TcAttachInfo{
				{
					InterfaceSelector: v1alpha1.InterfaceSelector{
						Interfaces: []string{"eth0"},
					},
					NetworkNamespaces: netNsSelector,
					Direction:         v1alpha1.TCIngress,
					Priority:          ptr.To(int32(55)),
				},
			},
		},
	}

	tcxProgram := v1alpha1.BpfApplicationProgram{
		Name: "tcx_stats",
		Type: v1alpha1.ProgTypeTCX,
		TCX: &v1alpha1.TcxProgramInfo{
			Links: []v1alpha1.TcxAttachInfo{
				{
					InterfaceSelector: v1alpha1.InterfaceSelector{
						Interfaces: []string{"eth0"},
					},
					NetworkNamespaces: netNsSelector,
					Direction:         v1alpha1.TCIngress,
					Priority:          ptr.To(int32(500)),
				},
			},
		},
	}

	newApp := func(namespace string, programs ...v1alpha1.BpfApplicationProgram) *v1alpha1.BpfApplication {
		return &v1alpha1.BpfApplication{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nsSameNameAppName,
				Namespace: namespace,
			},
			Spec: v1alpha1.BpfApplicationSpec{
				BpfAppCommon: v1alpha1.BpfAppCommon{
					NodeSelector: metav1.LabelSelector{},
					ByteCode: v1alpha1.ByteCodeSelector{
						Image: &v1alpha1.ByteCodeImage{
							Url: nsSameNameBytecode,
						},
					},
				},
				Programs: programs,
			},
		}
	}

	for _, ns := range []string{nsSameNameNs1, nsSameNameNs2} {
		t.Logf("creating namespace %s", ns)
		_, err := env.Cluster().Client().CoreV1().Namespaces().Create(ctx,
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		for _, ns := range []string{nsSameNameNs1, nsSameNameNs2} {
			cleanupLog("deleting BpfApplication in namespace %s", ns)
			bpfmanClient.BpfmanV1alpha1().BpfApplications(ns).Delete(ctx, nsSameNameAppName, metav1.DeleteOptions{})
		}
		// Wait for the agents to detach and remove their state objects
		// before deleting the namespaces, so the namespaces don't hang
		// on state-object finalizers.
		for _, ns := range []string{nsSameNameNs1, nsSameNameNs2} {
			require.Eventually(t, func() bool {
				states, err := bpfmanClient.BpfmanV1alpha1().BpfApplicationStates(ns).List(ctx, metav1.ListOptions{})
				return err == nil && len(states.Items) == 0
			}, 2*time.Minute, 2*time.Second)
			cleanupLog("deleting namespace %s", ns)
			env.Cluster().Client().CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
		}
	})

	// The first application must be fully reconciled before the
	// same-named second application appears, which is the ordering
	// that triggered the original failure.
	t.Logf("creating BpfApplication %s/%s with TC and TCX programs", nsSameNameNs1, nsSameNameAppName)
	_, err := bpfmanClient.BpfmanV1alpha1().BpfApplications(nsSameNameNs1).Create(ctx,
		newApp(nsSameNameNs1, tcProgram, tcxProgram), metav1.CreateOptions{})
	require.NoError(t, err)

	t.Logf("waiting for BpfApplication %s/%s to report its eBPF programs loaded and attached on all selected nodes (the Success status condition)",
		nsSameNameNs1, nsSameNameAppName)
	require.Eventually(t, namedBpfApplicationSuccess(t, nsSameNameNs1, nsSameNameAppName), 2*time.Minute, 5*time.Second)

	t.Logf("creating BpfApplication %s/%s with only a TC program", nsSameNameNs2, nsSameNameAppName)
	_, err = bpfmanClient.BpfmanV1alpha1().BpfApplications(nsSameNameNs2).Create(ctx,
		newApp(nsSameNameNs2, tcProgram), metav1.CreateOptions{})
	require.NoError(t, err)

	t.Logf("waiting for BpfApplication %s/%s to report its eBPF programs loaded and attached on all selected nodes (the Success status condition)",
		nsSameNameNs2, nsSameNameAppName)
	require.Eventually(t, namedBpfApplicationSuccess(t, nsSameNameNs2, nsSameNameAppName), 2*time.Minute, 5*time.Second)

	// The original failure oscillated the first application between
	// Success and Error as the agent repeatedly adopted its state
	// object on behalf of the second application, so a single
	// point-in-time check could pass by luck. Sample both applications
	// for a while to catch any flip.
	t.Logf("verifying %s/%s and %s/%s both keep reporting their eBPF programs loaded and attached (the Success status condition)",
		nsSameNameNs1, nsSameNameAppName, nsSameNameNs2, nsSameNameAppName)
	require.Never(t, func() bool {
		return !namedBpfApplicationSuccess(t, nsSameNameNs1, nsSameNameAppName)() ||
			!namedBpfApplicationSuccess(t, nsSameNameNs2, nsSameNameAppName)()
	}, 30*time.Second, 5*time.Second)

	// Each namespace must hold its own state objects (one per node),
	// owned by its own application and tracking that application's
	// program list, not the other namespace's.
	expectedPrograms := map[string]int{
		nsSameNameNs1: 2,
		nsSameNameNs2: 1,
	}
	var stateCounts []int
	for ns, numPrograms := range expectedPrograms {
		app, err := bpfmanClient.BpfmanV1alpha1().BpfApplications(ns).Get(ctx, nsSameNameAppName, metav1.GetOptions{})
		require.NoError(t, err)
		states, err := bpfmanClient.BpfmanV1alpha1().BpfApplicationStates(ns).List(ctx, metav1.ListOptions{})
		require.NoError(t, err)
		require.NotEmpty(t, states.Items, "expected BpfApplicationState objects in namespace %s", ns)
		stateCounts = append(stateCounts, len(states.Items))
		for _, state := range states.Items {
			owner := metav1.GetControllerOf(&state)
			require.NotNil(t, owner, "BpfApplicationState %s/%s should have a controller reference", ns, state.Name)
			require.Equal(t, app.UID, owner.UID,
				"BpfApplicationState %s/%s should be owned by its own namespace's application", ns, state.Name)
			require.Len(t, state.Status.Programs, numPrograms,
				"BpfApplicationState %s/%s should track its own application's programs", ns, state.Name)
			c := meta.FindStatusCondition(state.Status.Conditions, string(v1alpha1.BpfAppStateCondSuccess))
			require.NotNil(t, c, "BpfApplicationState %s/%s should have a Success condition", ns, state.Name)
			require.Equal(t, metav1.ConditionTrue, c.Status)
		}
	}

	require.Equal(t, stateCounts[0], stateCounts[1],
		"both namespaces should have one BpfApplicationState per node")
}

// namedBpfApplicationSuccess returns a function that checks if a
// namespaced BpfApplication has reached a successful state.
func namedBpfApplicationSuccess(t *testing.T, namespace, name string) func() bool {
	return func() bool {
		app, err := bpfmanClient.BpfmanV1alpha1().BpfApplications(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Logf("BpfApplication %s/%s not yet available: %v", namespace, name, err)
			return false
		}

		c := meta.FindStatusCondition(app.Status.Conditions, string(v1alpha1.BpfAppCondSuccess))
		if c == nil || c.Status != metav1.ConditionTrue {
			t.Logf("BpfApplication %s/%s eBPF programs not yet loaded and attached on all selected nodes: %+v",
				namespace, name, app.Status.Conditions)
			return false
		}

		return true
	}
}
