Create shoot namespace

```
cat ns-shoot.json
apiVersion: v1
kind: Namespace
metadata:
name: ns-shoot
```

```
# kubectl create -f ns-shoot.json
namespace/ns-shoot1 created
```

Create gc-kubeconfig secret in the namespace/ns-shoot1

Get Guest Cluster kubeconfig using SV Context
```
kubectl -n <gc-ns> get secret <gc-kubeconfig>  -o jsonpath='{.data.value}' | base64 -d > gc-kubeconfig.yaml
```

```
# kubectl create secret generic gc-kubeconfig --from-file=gc-kubeconfig.yaml --namespace=ns-shoot1
secret/gc-kubeconfig create
```

Switch to Guest Cluster context and export yaml for pvcsi-provider-creds and pvcsi-config


```
export KUBECONFIG=./gc-kubeconfig.yaml
kubectl get secret pvcsi-provider-creds --namespace=vmware-system-csi -o yaml > gc-pvcsi-provider-creds-secret.yaml
kubectl get configmap pvcsi-config --namespace=vmware-system-csi -o yaml > gc-pvcsi-config.yaml
```

Change namespace in gc-pvcsi-provider-creds-secret.yaml and gc-pvcsi-config.yaml to ns-shoot1 and create `pvcsi-provider-creds` secret and `pvcsi-config` configmap

```
kubectl apply -f gc-pvcsi-provider-creds-secret.yaml
secret/pvcsi-provider-creds created

# kubectl apply -f gc-pvcsi-config.yaml
configmap/pvcsi-config created
```

Deploy [pvcsi-seed.yaml](pvcsi-seed.yaml) in the ns-shoot namespace

Populate required fields in the [pvccsi-shoot.yaml](pvcsi-shoot.yaml) and deploy it in the SAP gardener seed cluster.





