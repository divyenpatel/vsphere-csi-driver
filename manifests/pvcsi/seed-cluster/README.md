# Prerequisites
- Create a namespace in the Seed Cluster for running vSphere CSI Controller.
  - In the PVCSI YAML files replace `{{ .PVCSISeedClusterNamespace }}` to namespace name.
- Deploy `pvcsi-provider-creds` secret in the `{{ .PVCSISeedClusterNamespace }}`.
   - `pvcsi-provider-creds` contains credential details like ca.crt, token and TKG namespace using which CSI controller in TKG can communicate with SV Cluster.
- Deploy `pvcsi-config` configmap in the `{{ .PVCSISeedClusterNamespace }}`.
   - `pvcsi-config` contains end point detail using which CSI controller can communicate with Supervisor Cluster.
- Deploy `pvcsi-kubeconfig` secret on the `{{ .PVCSISeedClusterNamespace }}`.
  - Get kubeconfig yaml of the target cluster.
    ```
    kubectl -n <tkg-ns> get secret <tkg-kubeconfig-secret-name>  -o jsonpath='{.data.value}' | base64 -d > kubeconfig.yaml
    ```
  - Create `pvcsi-kubeconfig` secret.
    ```
    kubectl create secret generic gc-kubeconfig --from-file=kubeconfig.yaml --namespace={{ .PVCSISeedClusterNamespace }}
    ```