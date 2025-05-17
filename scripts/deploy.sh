#!/bin/bash

# Deploy infrastructure
helm upgrade --install kafka charts/kafka -n messaging --values charts/kafka/values-production.yaml
helm upgrade --install mongodb charts/mongodb -n messaging --values charts/mongodb/values-production.yaml
helm upgrade --install redis charts/redis -n messaging --values charts/redis/values-production.yaml

# Wait for infrastructure
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=kafka -n messaging --timeout=300s
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=mongodb -n messaging --timeout=300s
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=redis -n messaging --timeout=300s

# Deploy application
helm upgrade --install messaging-app charts/app -n messaging --values charts/app/values-production.yaml

# Deploy monitoring
helm upgrade --install prometheus charts/prometheus -n monitoring
helm upgrade --install grafana charts/grafana -n monitoring

echo "Deployment completed"