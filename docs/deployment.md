
# Deployment Guide

This guide provides instructions for deploying the messaging application.

## Docker Compose

For local development and testing, you can use Docker Compose to run the application and its dependencies.

### Prerequisites

*   Docker
*   Docker Compose

### Running the Application

1.  **Build the images:**

    ```bash
    docker-compose build
    ```

2.  **Start the services:**

    ```bash
    docker-compose up -d
    ```

3.  **Stop the services:**

    ```bash
    docker-compose down
    ```

The `docker-compose.yml` file is configured to run the following services:

*   `app`: The messaging application itself.
*   `mongodb1`, `mongodb2`, `mongodb3`: MongoDB replica set.
*   `redis1`, `redis2`, `redis3`: Redis cluster.
*   `zookeeper`, `kafka1`, `kafka2`, `kafka3`: Kafka cluster.
*   `prometheus`: For metrics.
*   `grafana`: For dashboards.

## Kubernetes

### Using YAML Files

The `deployments` directory contains Kubernetes YAML files for deploying the application and its dependencies.

1.  **Deploy MongoDB:**

    ```bash
    kubectl apply -f deployments/mongodb.yaml
    ```

2.  **Deploy Redis:**

    ```bash
    kubectl apply -f deployments/redis.yaml
    ```

3.  **Deploy Kafka:**

    ```bash
    kubectl apply -f deployments/kafka.yaml
    ```

4.  **Deploy the Application:**

    You will need to build and push your application's Docker image to a registry that your Kubernetes cluster can access. Then, update the image name in a deployment file for the app (not provided, but you can create one based on the other deployments) and apply it.

### Using Helm

The `charts/app` directory contains a Helm chart for deploying the application.

1.  **Install the chart:**

    ```bash
    helm install my-release charts/app
    ```

You can customize the deployment by creating a `values.yaml` file and using it with the `helm install` command:

```bash
helm install my-release charts/app -f my-values.yaml
```

The default values can be found in `charts/app/values.yaml`.
