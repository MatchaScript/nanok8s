// Package staticpod generates the four static Pod manifests that kubelet
// picks up from /etc/kubernetes/manifests/ at boot: etcd, kube-apiserver,
// kube-controller-manager, and kube-scheduler.
//
// Manifests are rewritten only when their content differs from what is
// already on disk. Byte-equal manifests are left untouched so that kubelet
// does not restart containers on every `nanok8s apply` or reboot.
package staticpod

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	v1alpha1 "github.com/MatchaScript/nanok8s/internal/apis/bootstrap/v1alpha1"
)

const (
	fileMode = 0o644
	dirMode  = 0o755
)

// Layout lets tests redirect manifest output to a temp directory.
type Layout struct {
	ManifestsDir string
}

type Action string

const (
	ActionCreated Action = "created"
	ActionUpdated Action = "updated"
	ActionReused  Action = "reused"
)

type Report struct {
	Items []ReportItem
}

type ReportItem struct {
	ID     string
	File   string
	Action Action
}

func (r *Report) add(id, file string, action Action) {
	r.Items = append(r.Items, ReportItem{ID: id, File: file, Action: action})
}

// Ensure renders all four static pod manifests. nodeName is embedded in the
// etcd member name and in the etcd pod name, matching kubeadm's convention.
func Ensure(cfg *v1alpha1.NanoK8sConfig, layout Layout, nodeName string) (*Report, error) {
	manifests := []struct {
		id       string
		filename string
		obj      *pod
	}{
		{"etcd", "etcd.yaml", etcdPod(cfg, nodeName)},
		{"kube-apiserver", "kube-apiserver.yaml", apiserverPod(cfg)},
		{"kube-controller-manager", "kube-controller-manager.yaml", controllerManagerPod(cfg)},
		{"kube-scheduler", "kube-scheduler.yaml", schedulerPod()},
	}

	report := &Report{}
	for _, m := range manifests {
		data, err := yaml.Marshal(m.obj)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", m.id, err)
		}
		path := filepath.Join(layout.ManifestsDir, m.filename)
		action, err := writeIfChanged(path, data)
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
		report.add(m.id, path, action)
	}
	return report, nil
}

// writeIfChanged writes data to path if it differs from the current file
// content (or the file is missing). A byte-equal existing file is left alone
// so kubelet never sees a meaningless manifest change.
func writeIfChanged(path string, data []byte) (Action, error) {
	existing, err := os.ReadFile(path)
	if err == nil {
		if bytes.Equal(existing, data) {
			return ActionReused, nil
		}
		if err := os.WriteFile(path, data, fileMode); err != nil {
			return "", err
		}
		return ActionUpdated, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, fileMode); err != nil {
		return "", err
	}
	return ActionCreated, nil
}
