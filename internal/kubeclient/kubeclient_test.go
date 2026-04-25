package kubeclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

// newFakeAPIServer spins an httptest server and returns a clientset wired
// to it. The routes map is keyed by URL path and returns the object to
// marshal as JSON. A handler returning 503 is used for /readyz until
// `ready` flips.
type fakeServer struct {
	t      *testing.T
	ready  atomic.Bool
	mu     sync.Mutex
	nodes  map[string]*corev1.Node
	pods   map[string]*corev1.Pod
	server *httptest.Server
	client kubernetes.Interface
}

func newFakeAPIServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{
		t:     t,
		nodes: map[string]*corev1.Node{},
		pods:  map[string]*corev1.Pod{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)

	cs, err := kubernetes.NewForConfig(&restclient.Config{Host: f.server.URL})
	if err != nil {
		t.Fatalf("build clientset: %v", err)
	}
	f.client = cs
	return f
}

func (f *fakeServer) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.URL.Path == "/readyz":
		if f.ready.Load() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return

	case r.Method == http.MethodGet && matchNode(r.URL.Path) != "":
		name := matchNode(r.URL.Path)
		f.mu.Lock()
		n, ok := f.nodes[name]
		f.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, n)
		return

	case r.Method == http.MethodGet && matchPod(r.URL.Path) != "":
		name := matchPod(r.URL.Path)
		f.mu.Lock()
		p, ok := f.pods[name]
		f.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, p)
		return
	}

	http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
}

// matchNode returns "" unless path matches /api/v1/nodes/{name}.
func matchNode(path string) string {
	const prefix = "/api/v1/nodes/"
	if len(path) > len(prefix) && path[:len(prefix)] == prefix {
		return path[len(prefix):]
	}
	return ""
}

// matchPod returns "" unless path matches /api/v1/namespaces/kube-system/pods/{name}.
func matchPod(path string) string {
	const prefix = "/api/v1/namespaces/kube-system/pods/"
	if len(path) > len(prefix) && path[:len(prefix)] == prefix {
		return path[len(prefix):]
	}
	return ""
}

func writeJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (f *fakeServer) setNodeReady(name string, ready bool) {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes[name] = &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: status},
			},
		},
	}
}

func (f *fakeServer) setPodReady(name string, ready bool) {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pods[name] = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "kube-system"},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: status},
			},
		},
	}
}

func TestWaitForAPIServer_ReturnsWhenReadyz200s(t *testing.T) {
	f := newFakeAPIServer(t)
	f.ready.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitForAPIServer(ctx, f.client); err != nil {
		t.Fatalf("WaitForAPIServer: %v", err)
	}
}

func TestWaitForAPIServer_PollsUntilReady(t *testing.T) {
	f := newFakeAPIServer(t)
	go func() {
		time.Sleep(300 * time.Millisecond)
		f.ready.Store(true)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if err := WaitForAPIServer(ctx, f.client); err != nil {
		t.Fatalf("WaitForAPIServer: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Errorf("WaitForAPIServer returned suspiciously fast (%v); fake was not ready yet", elapsed)
	}
}

func TestWaitForAPIServer_TimeoutSurfacesLastError(t *testing.T) {
	f := newFakeAPIServer(t)
	// ready stays false forever

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := WaitForAPIServer(ctx, f.client)
	if err == nil {
		t.Fatal("WaitForAPIServer on never-ready apiserver = nil")
	}
}

func TestWaitForControlPlane_AllReadyReturns(t *testing.T) {
	f := newFakeAPIServer(t)
	const node = "cp-1"
	f.setNodeReady(node, true)
	f.setPodReady("kube-apiserver-"+node, true)
	f.setPodReady("kube-controller-manager-"+node, true)
	f.setPodReady("kube-scheduler-"+node, true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := WaitForControlPlane(ctx, f.client, node); err != nil {
		t.Fatalf("WaitForControlPlane: %v", err)
	}
}

// If even one static pod never reaches Ready the call must time out.
// Mirrors the production case where kube-controller-manager crash-loops
// while /readyz on the apiserver itself stays green.
func TestWaitForControlPlane_MissingOnePodTimesOut(t *testing.T) {
	f := newFakeAPIServer(t)
	const node = "cp-1"
	f.setNodeReady(node, true)
	f.setPodReady("kube-apiserver-"+node, true)
	f.setPodReady("kube-controller-manager-"+node, false) // stuck not-ready
	f.setPodReady("kube-scheduler-"+node, true)

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	err := WaitForControlPlane(ctx, f.client, node)
	if err == nil {
		t.Fatal("WaitForControlPlane with not-ready CM = nil; want timeout")
	}
}

// Node reports NodeReady=False (kubelet running but CNI not up). Every
// other pod is Ready but the overall check must still block.
func TestWaitForControlPlane_NodeNotReadyTimesOut(t *testing.T) {
	f := newFakeAPIServer(t)
	const node = "cp-1"
	f.setNodeReady(node, false)
	f.setPodReady("kube-apiserver-"+node, true)
	f.setPodReady("kube-controller-manager-"+node, true)
	f.setPodReady("kube-scheduler-"+node, true)

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	if err := WaitForControlPlane(ctx, f.client, node); err == nil {
		t.Fatal("WaitForControlPlane with NodeReady=False = nil; want timeout")
	}
}

// Pod becomes Ready partway through — must be picked up without the
// caller needing to re-invoke.
func TestWaitForControlPlane_EventuallyReadyPasses(t *testing.T) {
	f := newFakeAPIServer(t)
	const node = "cp-1"
	f.setNodeReady(node, true)
	f.setPodReady("kube-apiserver-"+node, true)
	f.setPodReady("kube-controller-manager-"+node, false)
	f.setPodReady("kube-scheduler-"+node, true)

	go func() {
		time.Sleep(300 * time.Millisecond)
		f.setPodReady("kube-controller-manager-"+node, true)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := WaitForControlPlane(ctx, f.client, node); err != nil {
		t.Fatalf("WaitForControlPlane: %v", err)
	}
}

func TestLoadAdmin_InvalidPathFails(t *testing.T) {
	_, err := LoadAdmin("/nonexistent/admin.conf")
	if err == nil {
		t.Fatal("LoadAdmin on missing file = nil")
	}
}
