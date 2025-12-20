# Messaging Platform Kubernetes Stack

This directory contains production-oriented Kubernetes manifests that replace the local `docker-compose.yml` stack.  The manifests assume two namespaces (`messaging` for the application and data plane, `monitoring` for observability) and rely on a fast block storage class named `fast-ssd` (update as needed for your cluster).

## Components

The stack includes:

1. **MongoDB replica set** with init job and metrics sidecar.
2. **Redis six-node cluster** (3 masters + 3 replicas) and initializer job.
3. **Apache Kafka** (3 brokers) backed by a 3-node Zookeeper ensemble and topic bootstrap job.
4. **Cassandra** three-node ring for chat persistence.
5. **Neo4j** single primary instance with persistent storage.
6. **MinIO** distributed deployment (4 nodes) with a bucket bootstrap job.
7. **Messaging API** deployment (Go service) with autoscaling, network ingress, and ConfigMap-driven configuration.
8. **Exporter pods** for MongoDB/Redis plus placeholders for hooking into an existing Prometheus/Grafana stack.

> **Secrets** – All sensitive values live in `secrets-example.yaml`. Copy it to `secrets.yaml`, replace the placeholder/base64 values, and keep the real file out of source control.

## Usage

1. **Apply namespaces and secrets**
   ```bash
   kubectl apply -f k8s/namespaces.yaml
   cp k8s/secrets-example.yaml k8s/secrets.yaml   # edit values, never commit this file
   kubectl apply -f k8s/secrets.yaml
   ```
2. **Deploy the stack**
   ```bash
   kubectl apply -k k8s
   ```
3. **Watch rollout**
   ```bash
   kubectl get pods -n messaging -w
   ```

## Validation checklist

- Ensure StatefulSets report all pods `Ready`:
  `kubectl get sts -n messaging`.
- Confirm init Jobs succeeded (MongoDB replica set, Redis cluster, Kafka topic, MinIO buckets):
  `kubectl get jobs -n messaging`.
- Verify the messaging API answers `/health` and Prometheus picks up exporters.

## Requirements and notes

- A `StorageClass` named `fast-ssd` must exist (or update each PVC with your class).
- Provide a LoadBalancer/Ingress controller for `api.example.com` and MinIO access.
- Prometheus Operator CRDs are expected for the ServiceMonitor objects.
- Update `ghcr.io/your-org/messaging-app:latest` to the registry/tag you publish from CI.
- Replace hostnames/CORS domains inside `app-config.yaml` with your production values.
- Consider layering PodDisruptionBudgets and NetworkPolicies per your security posture.

Refer to inline comments for resource tuning guidance and integrate with your preferred GitOps flow (Argo CD, Flux, etc.). Update hostnames/ingress TLS settings to match your environment.
