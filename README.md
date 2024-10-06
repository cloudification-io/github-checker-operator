# Github Checker Operator

This project implements a Kubernetes operator written in Golang using Kubebuilder framework that manages custom resources of type `Checker`. The operator handles the creation and management of related resources and continuously monitors pod logs to update the status of the `Checker` resource.

## Table of Contents

- [Project Overview](#project-overview)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
- [How the Operator Works](#how-the-operator-works)
- [Usage](#usage)

## Project Overview

The `Checker` operator provides the following functionalities:

1. **Resource Management**: Automatically creates `ConfigMaps` and `CronJobs` based on the definition of a `Checker` custom resource.
2. **Pod Log Monitoring**: The operator tracks the logs of the latest pod created by the CronJob, evaluates the status from the logs, and updates the `Checker` status accordingly.
3. **RBAC Permissions**: RBAC roles and permissions are defined using static manifests, ensuring the operator has the necessary access to manage resources in the cluster.

## Getting Started

### Prerequisites

- A Kubernetes cluster (local or remote).
- `kubectl` CLI installed and configured to manage the cluster.
- Docker installed for building and pushing container images.
- Golang (version 1.22 or later).

### Installation

1. **Deploy the Operator**:

    Use the static Kubernetes manifest to deploy the operator in your cluster:

    ```bash
    kubectl apply -f example/operator.yaml
    ```

2. **Create a `Checker` Resource**:

    The operator will automatically generate associated resources. To create an instance of the `Checker` resource:

    ```bash
    kubectl apply -f example/example.yaml
    ```

## How the Operator Works

- **Checker Custom Resource**: When a `Checker` resource is created, the operator generates a corresponding `ConfigMap` and `CronJob` based on the resource's configuration.
- **Pod Log Monitoring**: The operator monitors the logs of the latest pod created by the CronJob. If the logs indicate a status code in the 2XX, 3XX, 4XX, or 5XX range, it updates the `Checker` resource's status accordingly.
- **Dynamic Status Updates**: The status of the `Checker` resource is continuously updated based on the latest pod logs, providing real-time feedback.

## Usage

1. **Check the `Checker` Status**: After creating the `Checker` resource, you can check its status by running:

    ```bash
    kubectl get checkers
    ```

2. **Monitor Logs**: The operator captures the logs of the pods created by the CronJob and automatically updates the `Checker` status.

3. **Customize the CronJob**: You can modify the `Checker` custom resource spec to adjust the configuration of the `ConfigMap` and the associated `CronJob`.
