
# Local Development Guide

This guide explains how to set up the messaging application for local development.

## Prerequisites

*   Go (version 1.18 or higher)
*   Docker
*   Docker Compose
*   Make

## Setup and Running

The project uses a `Makefile` to simplify common tasks.

1.  **Build the Docker images:**

    ```bash
    make build
    ```

2.  **Start all services in the background:**

    ```bash
    make up
    ```

    This will start the application, MongoDB, Redis, and Kafka using `docker-compose`.

3.  **Stop all services:**

    ```bash
    make down
    ```

## Running Tests

To run the test suite:

```bash
make test
```

## Makefile Commands

*   `make build`: Build the Docker images for all services.
*   `make up`: Start all services in detached mode.
*   `make down`: Stop and remove all running containers.
*   `make test`: Run the Go tests.
*   `make benchmark`: Run benchmark tests.
*   `make deploy`: Run the deployment script.
*   `make monitor`: Open the Grafana dashboard in your browser.
