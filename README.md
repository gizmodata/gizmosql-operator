# ☸️ GizmoSQL Kubernetes Operator

[![DockerHub](https://img.shields.io/badge/dockerhub-image-green.svg?logo=Docker)](https://hub.docker.com/r/gizmodata/gizmosql-operator)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

---

## 🌟 What is the GizmoSQL Operator?

The **GizmoSQL Operator** is a Kubernetes controller that manages the lifecycle of [GizmoSQL](https://github.com/gizmodata/gizmosql) instances on Kubernetes. It simplifies the deployment, scaling, and management of high-performance SQL servers powered by DuckDB.

---

## 🧠 Why use the Operator?

- 🤖 **Automated Lifecycle** — Easily provision and manage GizmoSQL instances as Kubernetes Custom Resources.
- ⚙️ **Configuration Management** — Declarative configuration for your SQL engines.
- 🚀 **Scalable Deployments** — Deploy multiple isolated instances with ease.
- ☁️ **Cloud Native** — Integrates with Kubernetes RBAC, networking, and storage.

---

## 🚀 Quick Start

### Prerequisites

- Kubernetes cluster (v1.11.3+)
- `kubectl` configured
- `helm` (v3+)

### Installation via Helm

1. **Install the Chart**

   Install the operator into the `gizmosql-system` namespace:

   ```bash
   helm install gizmosql-operator oci://registry-1.docker.io/gizmodata/gizmosql-operator-chart \
     --namespace gizmosql-system \
     --create-namespace
   ```

2. **Verify Installation**

   Wait until the operator pod is running:

   ```bash
   kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=gizmosql-operator --namespace gizmosql-system
   ```

---

## 📦 Usage

Once the operator is running, you can deploy a GizmoSQL instance by creating a `GizmoSQLServer` custom resource.

### 1. Create a Manifest

Create a file named `gizmosql-instance.yaml`:

```yaml
apiVersion: gizmodata.com/v1alpha1
kind: GizmoSQLServer
metadata:
  name: example-gizmosql
  namespace: default
spec:
  resources:
    limits:
      cpu: "1"
      memory: "2Gi"
    requests:
      cpu: "500m"
      memory: "1Gi"
```

### 2. Apply the Manifest

```bash
kubectl apply -f gizmosql-instance.yaml
```

### 3. Connect

The operator will create a Service for your instance. You can forward the port to connect locally:

```bash
kubectl port-forward svc/example-gizmosql 31337:31337
```

Then connect using the GizmoSQL client or any Arrow Flight SQL compatible client (JDBC, Python, etc.).

---

## 🛠 Configuration

The Helm chart can be configured via `values.yaml` or `--set` flags during installation.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `manager.image.repository` | Controller image repository | `gizmodata/gizmosql-operator` |
| `manager.image.tag` | Controller image tag | `0.1.0` |
| `manager.replicas` | Number of operator replicas | `1` |
| `crd.enable` | Whether to install CRDs | `true` |
| `metrics.enable` | Enable metrics endpoint | `true` |

---

## 🏗️ Build from Source

For developers who want to build and deploy the operator from source:

Download and install [k3d](https://k3d.io/) by following the official instructions. For most platforms, a simple install script is available:

```bash
curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
```

Create a container registry to push your images to:

```bash
k3d registry create gizmodata
```

Create a new cluster with the registry:

```bash
k3d cluster create --registry-use k3d-gizmodata:63806 gizmodata
```


```bash
# Build the docker image
make docker-build-and-push-dev

# Deploy via Helm with your custom image
helm upgrade --install gizmosql-operator \
  --namespace gizmosql-system \
  --create-namespace ./chart \
  --set manager.image.repository=k3d-gizmodata:63806/gizmosql-operator \
  --set manager.image.tag=$(docker images | grep -m1 dev. | awk '{print $2}')
```

---

## 🔒 License


```
Apache License, Version 2.0
https://www.apache.org/licenses/LICENSE-2.0
```

---

## 📫 Contact

Questions or consulting needs?

📧 info@gizmodata.com  
🌐 [https://gizmodata.com](https://gizmodata.com)

---

> Built with ❤️ by [GizmoData™](https://gizmodata.com)
