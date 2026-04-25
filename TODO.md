# TODO

未対応項目。基本方針: **kubeadm canonical な挙動に full で揃える**。「single-node なので」「v0 なので最低限」を根拠に独自の劣化版を作らない。設定項目で吸収できる差分は config に expose する。

---

## HIGH

### 1. admin.conf の RBAC 問題を kubeadm canonical な方法で解決

**Where:** [internal/kubeadm/ensure.go](internal/kubeadm/ensure.go) / [internal/lifecycle/boot.go](internal/lifecycle/boot.go) の readyz 到達後

**実装内容:**
1. `Ensure` で `super-admin.conf` も併せて生成 — `kubeconfig.CreateKubeConfigFile(kubeadmconstants.SuperAdminKubeConfigFileName, ...)` を追加呼び出し
2. readyz 後に `kubeconfig.EnsureAdminClusterRoleBinding(layout.KubeconfigDir, nil)` を呼ぶ
   - 内部動作: まず `admin.conf` で `kubeadm:cluster-admins` CRB 作成を試み、403 なら `super-admin.conf` に fallback して CRB を作る ([kubeconfig.go:650-758](reference/microshift/deps/github.com/openshift/kubernetes/cmd/kubeadm/app/phases/kubeconfig/kubeconfig.go#L650-L758))
   - これは kubeadm init が内部で呼んでいる挙動そのもの ([init.go:552-553](reference/microshift/deps/github.com/openshift/kubernetes/cmd/kubeadm/app/cmd/init.go#L552-L553))

**Why:**
- kubeadm v1.29+ では `admin.conf` は `kubeadm:cluster-admins` Group 向けに発行され、対応する `ClusterRoleBinding` が無いと `kubectl --kubeconfig=/etc/kubernetes/admin.conf` が unauthorized になる
- `super-admin.conf` は `system:masters` Group (built-in cluster-admin bypass) なので、CRB 作成の bootstrap としてのみ使う

**`upload-config` / `bootstrap-token` phase は呼ばない (legitimate に不要):**
- `upload-config` が書く `kube-system/kubeadm-config` + `kubelet-config` ConfigMap は **`kubeadm upgrade` / `kubeadm join` CLI の consumer にしか使われない**
- `bootstrap-token` phase が作る token + RBAC は `kubeadm join` の node discovery 専用
- nanok8s は bootc image swap + Ensure 再実行で upgrade、join は v0 対象外。consumer が存在しない phase を呼ぶ意味が無い
- Talos ([reference/talos/internal/app/machined/pkg/controllers/k8s/](reference/talos/internal/app/machined/pkg/controllers/k8s/)) / vcluster — 両方同じ理由で skip。microshift は vendor only で未呼び出し

---

### 2. mark-control-plane phase を呼ぶ + `nodeRegistration.taints` を config に expose

**Where:**
- [internal/apis/bootstrap/v1alpha1/types.go](internal/apis/bootstrap/v1alpha1/types.go), [defaults.go](internal/apis/bootstrap/v1alpha1/defaults.go) — config field 追加
- [internal/kubeadm/config.go](internal/kubeadm/config.go) — `BuildInitConfiguration` で反映
- [internal/kubeadm/ensure.go](internal/kubeadm/ensure.go) — readyz 後に `markcontrolplane.MarkControlPlane(client, nodeName, taints)` 呼び出し

**実装内容:**
- `spec.nodeRegistration.taints` (type: `[]corev1.Taint`) を config に追加
- default は kubeadm と同じ `node-role.kubernetes.io/control-plane:NoSchedule`
- user が `[]` を明示指定すれば taint 無し (workload を CP に載せたいケース)
- `BuildInitConfiguration` で `kc.NodeRegistration.Taints = cfg.Spec.NodeRegistration.Taints`
- mark-control-plane phase を呼んで label (`node-role.kubernetes.io/control-plane`) + taint を Node に適用

**Why:**
- label は addon の nodeSelector / toleration が前提にしているので必要
- taint は deployment の性質 (workloads on CP か否か) で決まる設定項目。hardcode 分岐にせず config field で吸収

**参照:** [reference/microshift/deps/.../markcontrolplane/markcontrolplane.go:37-52](reference/microshift/deps/github.com/openshift/kubernetes/cmd/kubeadm/app/phases/markcontrolplane/markcontrolplane.go#L37-L52)

---

### 3. waitReadyz を kubeadm 流の多段 check に揃える

**Where:** [internal/kubeclient/kubeclient.go](internal/kubeclient/kubeclient.go), [internal/lifecycle/boot.go](internal/lifecycle/boot.go)

**実装内容 (kubeadm kinder と同じ 4 条件):**
- Node が `Ready=True`
- Static pod `kube-apiserver-<hostname>` が `Ready=True`
- Static pod `kube-controller-manager-<hostname>` が `Ready=True`
- Static pod `kube-scheduler-<hostname>` が `Ready=True`

並列 goroutine で polling、すべて満たすまで待つ。timeout は現行 3 分を据え置き。

**Why:**
- 現行 `/readyz` 一本だと CM / scheduler が crash-loop でも healthy 判定される → 壊れた state を last-boot に記録してしまう
- 参照: [reference/kubeadm/kinder/pkg/cluster/manager/actions/waiter.go:34-46, 131-195](reference/kubeadm/kinder/pkg/cluster/manager/actions/waiter.go#L34-L46)

---

### 4. reset を kubeadm reset full に揃える

**Where:** [cmd/nanok8s/reset.go](cmd/nanok8s/reset.go)

**実装内容 (kubeadm reset と同じ):**
1. `systemctl stop kubelet` (running なら)
2. `crictl` 経由で残存コンテナ stop + rm
3. `/etc/kubernetes/` 削除
4. `/var/lib/etcd/` 削除
5. `/var/lib/kubelet/` 削除
6. `/var/lib/nanok8s/` 削除 (nanok8s 独自)
7. CNI interfaces 削除 (`cni0`, `flannel.1` 等が存在すれば `ip link delete`)
8. iptables rules 削除 (`iptables -F`, `iptables -t nat -F`, `iptables -t mangle -F`, `iptables -X` + ipvs ruleset)

**Why:**
- 現行は 3/4/6 のみ。CNI / iptables を残すと次の bootstrap が干渉する
- 「CNI は scope 外」は cluster 運用時の話であって、**reset (= tear down) では kubeadm と同じく全掃除するのが正解**

---

## MED

### 5. bootstrap refusal を実効化

**Where:** [cmd/nanok8s/bootstrap.go:33-40](cmd/nanok8s/bootstrap.go#L33-L40), [internal/state/state.go](internal/state/state.go)

**Why:**
- docstring は「Refuses to run if nanok8s state already exists」だが `state.Exists()` は Boot 成功時にしか書かれないファイルしか見ないので効かない
- kubeadm init は preflight で `DirAvailable--etc-kubernetes-manifests` をチェックする — それに揃える

**実装方針:**
- `state.Exists()` に `/etc/kubernetes/manifests/kube-apiserver.yaml` の存在チェックを追加
- あわせて bootstrap 完了時にも `state.WriteLastEvent("bootstrapped at <version>")` を書く (観測可能性のため)

---

### 6. 旧 `nanok8s-apply.service` / `.timer` を削除

**Where:** [packaging/systemd/nanok8s-apply.service](packaging/systemd/nanok8s-apply.service), [packaging/systemd/nanok8s-apply.timer](packaging/systemd/nanok8s-apply.timer)

**Why:**
- `/usr/bin/nanok8s apply` を参照するが `apply` subcommand は pivot 時に削除済み
- install 時に enable すると確実に fail
- pivot 後の lifecycle は `nanok8s.service` (oneshot) 1 本のみ

---

### 7. README を現行コマンド体系に書き直す

**Where:** [README.md](README.md)

**書き直しポイント:**
- L29-37 Image contents: `nanok8s-apply.service` / `.timer` → `nanok8s.service`、greenboot drop-in 一式 (`required.d`, `wanted.d`, `red.d`) を追記
- L62-87 First-boot flow: 現行は `bootstrap → systemctl enable --now nanok8s.service` の 2 ステップ
- L104-112 Commands テーブル: `nanok8s apply` / `nanok8s addons apply` を削除

---

## LOW

### 8. version.go の未使用 image 定数を削除

**Where:** [internal/version/version.go:18-30](internal/version/version.go#L18-L30)

`EtcdImage` / `PauseImage` / `CoreDNSImage` / `ImageFor()` は呼び出しゼロ。kubeadm phase が `ClusterConfiguration.ImageRepository` から解決するので nanok8s 側では不要。

---

### 9. `config.KubernetesVersion` field を削除

**Where:** [internal/apis/bootstrap/v1alpha1/types.go:29](internal/apis/bootstrap/v1alpha1/types.go#L29) / [defaults.go:25-27](internal/apis/bootstrap/v1alpha1/defaults.go#L25-L27) / [validate.go:25-30](internal/apis/bootstrap/v1alpha1/validate.go#L25-L30) / [internal/kubeadm/config.go:53](internal/kubeadm/config.go#L53)

**実装内容:**
- `NanoK8sConfigSpec.KubernetesVersion` field を削除
- `SetDefaults` の補完と `Validate` の一致チェックも削除
- `BuildInitConfiguration` では `version.KubernetesVersion` を直接使う
- yaml 側でこの field を指定してきたら `UnmarshalStrict` が unknown field で reject するので罠も消える

**Why:** user から override できない tautological field 。kubeadm の `InitConfiguration.KubernetesVersion` の借用だが、nanok8s は minor pin 運用なので意味が無く、`v1.36.0` を `v1.35.0` binary に食わせるとエラーになるだけの罠フィールド。

---

### 10. 各 cobra cmd に `Args: cobra.NoArgs` を追加

**Where:** [cmd/nanok8s/root.go](cmd/nanok8s/root.go), [bootstrap.go](cmd/nanok8s/bootstrap.go), [boot.go](cmd/nanok8s/boot.go), [reset.go](cmd/nanok8s/reset.go), [config.go](cmd/nanok8s/config.go)

kubeadm 本家は全 cmd に `Args: cobra.NoArgs` を明示。揃える。

---

### 11. 削除済み subcommand を参照する stale comment を修正

- [internal/kubeadm/ensure.go:24](internal/kubeadm/ensure.go#L24) 「after `nanok8s apply`」
- [internal/apis/bootstrap/v1alpha1/types.go:2](internal/apis/bootstrap/v1alpha1/types.go#L2) 「and `nanok8s apply`」
- [internal/kubeclient/kubeclient.go:30-33](internal/kubeclient/kubeclient.go#L30-L33) 「Intended for use by `nanok8s addons apply`」

現行 (`nanok8s.service` / 内部 `boot` subcommand / `lifecycle.Boot`) に書き換える。

---

### 12. cert 期限チェック + 自動再生

**Where:** [internal/kubeadm/ensure.go](internal/kubeadm/ensure.go) の certs.CreatePKIAssets 周辺

kubeadm は `kubeadm certs renew` として別コマンド化しているが、nanok8s は oneshot 前提なので boot 時に inline で leaf cert の NotAfter をチェックして閾値内なら regenerate する (vcluster 方式)。

**参照:** [reference/vcluster/pkg/certs/ensure.go:100-144](reference/vcluster/pkg/certs/ensure.go#L100-L144)

---

## 着手順メモ

- **1, 2, 3** は post-apiserver phase 群なので 1 コミットで揃える方が筋が良い
- **4, 5** は独立
- **6, 7** は旧構成の掃除、まとめて片付ける
- **8, 9, 10, 11** は細かい整理、まとめて 1 コミットで OK
- **12** は cert validity の見直しと合わせて別途
