//go:build integration_tests
// +build integration_tests

package integration

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/kong/kubernetes-testing-framework/pkg/clusters"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	tcxGoCounterKustomize       = "https://github.com/bpfman/bpfman/examples/config/default/go-tcx-counter/?timeout=120&ref=main"
	tcxGoCounterUserspaceNs     = "go-tcx-counter"
	tcxGoCounterUserspaceDsName = "go-tcx-counter-ds"
)

func TestTcxGoCounter(t *testing.T) {
	t.Log("deploying tcx counter program")
	require.NoError(t, clusters.KustomizeDeployForCluster(ctx, env.Cluster(), tcxGoCounterKustomize))
	addCleanup(func(context.Context) error {
		cleanupLog("cleaning up tcx counter program")
		return clusters.KustomizeDeleteForCluster(ctx, env.Cluster(), tcxGoCounterKustomize)
	})

	t.Log("waiting for go tcx counter userspace daemon to be available")
	require.Eventually(t, func() bool {
		daemon, err := env.Cluster().Client().AppsV1().DaemonSets(tcxGoCounterUserspaceNs).Get(ctx, tcxGoCounterUserspaceDsName, metav1.GetOptions{})
		require.NoError(t, err)
		return daemon.Status.DesiredNumberScheduled == daemon.Status.NumberAvailable
	},
		// Wait 5 minutes since cosign is slow, https://github.com/bpfman/bpfman/issues/1043
		5*time.Minute, 10*time.Second)

	pods, err := env.Cluster().Client().CoreV1().Pods(tcxGoCounterUserspaceNs).List(ctx, metav1.ListOptions{LabelSelector: "name=go-tcx-counter"})
	require.NoError(t, err)
	gotcxCounterPod := pods.Items[0]

	req := env.Cluster().Client().CoreV1().Pods(tcxGoCounterUserspaceNs).GetLogs(gotcxCounterPod.Name, &corev1.PodLogOptions{})

	require.Eventually(t, func() bool {
		logs, err := req.Stream(ctx)
		require.NoError(t, err)
		defer logs.Close()
		output := new(bytes.Buffer)
		_, err = io.Copy(output, logs)
		require.NoError(t, err)
		t.Logf("counter pod log %s", output.String())

		return doTcxCheck(t, output)
	}, 30*time.Second, time.Second)
}
