
# Messaging App Backend

![Build Status](https://img.shields.io/badge/build-passing-brightgreen)
![Code Coverage](https://img.shields.io/badge/coverage-85%25-brightgreen)
![License](https://img.shields.io/badge/license-MIT-blue)

This repository contains the backend for a scalable, real-time messaging application. It is designed to be highly available and performant, using a modern, cloud-native architecture.

## Features

*   **Real-time Messaging:** One-to-one and group messaging with WebSocket.
*   **Authentication:** Secure user registration and login with JWT.
*   **Friendship System:** Send, accept, and manage friend requests.
*   **Group Management:** Create and manage groups, add/remove members, and assign admins.
*   **Scalable Architecture:** Built with microservices principles, using Kafka for message queuing and Redis for caching.
*   **Observability:** Integrated with Prometheus and Grafana for monitoring and metrics.

## Tech Stack

*   **Language:** Go
*   **Framework:** Gin
*   **Database:** MongoDB (Replica Set)
*   **Message Broker:** Kafka
*   **Cache:** Redis (Cluster)
*   **Real-time Communication:** WebSocket
*   **Containerization:** Docker, Docker Compose
*   **Deployment:** Kubernetes, Helm
*   **Monitoring:** Prometheus, Grafana

## Getting Started

To get a local copy up and running, follow these simple steps.

### Prerequisites

*   Go (version 1.18 or higher)
*   Docker
*   Docker Compose
*   Make

### Installation

1.  **Clone the repo:**

    ```bash
    git clone https://github.com/your_username/messaging-app.git
    ```

2.  **Build and start the services:**

    ```bash
    make up
    ```

    This will build the Docker images and start all the necessary services in the background.

## Documentation

For more detailed information, please refer to our documentation:

*   [**Business Logic and User Stories**](./docs/business_logic.md)
*   [**API Documentation**](./docs/api.md)
*   [**Deployment Guide**](./docs/deployment.md)
*   [**Local Development Guide**](./docs/local_development.md)

## Contributing

Contributions are what make the open-source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**.

1.  Fork the Project
2.  Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3.  Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4.  Push to the Branch (`git push origin feature/AmazingFeature`)
5.  Open a Pull Request

## License

Distributed under the MIT License. See `LICENSE` for more information.
