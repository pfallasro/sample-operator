# Kubernetes Operator Pattern Example

A practical demonstration of a Kubernetes operator that manages a custom `WebApp` resource. This example showcases the core concepts of Custom Resource Definitions (CRDs) and the reconciliation loop pattern.

## What is a Kubernetes Operator?

A Kubernetes operator is a software extension that uses **Custom Resources** to manage applications and their components. Operators follow Kubernetes principles, notably the **control loop** pattern, to automate operational tasks that would otherwise require human intervention.

**Key Concept**: Operators codify human operational knowledge into software that automatically manages complex applications.

## Core Concepts

### 1. Custom Resource Definition (CRD)

A CRD extends the Kubernetes API to include custom resource types. In this example, we define a `WebApp` resource.

**File**: [crd/webapp-crd.yaml](crd/webapp-crd.yaml)

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: webapps.example.com
spec:
  group: example.com
  names:
    kind: WebApp
    plural: webapps
```

**What it does**:
- Defines a new resource type `WebApp` in the `example.com` API group
- Specifies the schema (what fields are allowed)
- Enables validation (e.g., replicas must be 1-10)
- Defines status subresource for tracking application state

### 2. Custom Resource (CR)

Once the CRD is installed, you can create instances of the custom resource.

**File**: [examples/nginx-webapp.yaml](examples/nginx-webapp.yaml)

```yaml
apiVersion: example.com/v1
kind: WebApp
metadata:
  name: nginx-app
spec:
  image: nginx:1.25
  replicas: 3
  port: 80
```

This is a declarative specification: "I want an nginx web app with 3 replicas."

### 3. The Reconciliation Loop

The operator continuously watches for changes to `WebApp` resources and reconciles the actual state with the desired state.

**File**: [main.go](main.go) - See the `Reconcile()` function

#### How the Reconciliation Loop Works

```
┌─────────────────────────────────────────────────┐
│  User creates/updates WebApp CR                │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│  1. Operator watches for WebApp events          │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│  2. Reconcile() function is triggered           │
│     - Fetch the WebApp resource                 │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│  3. Check if Deployment exists                  │
│     - If not, create it                         │
│     - If yes, check if it matches spec          │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│  4. Check if Service exists                     │
│     - If not, create it                         │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│  5. Update WebApp status                        │
│     - Available replicas count                  │
│     - Ready condition                           │
└─────────────────┬───────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────┐
│  6. Requeue after 30s                           │
│     (Loop continues)                            │
└─────────────────────────────────────────────────┘
```

#### Key Principles of Reconciliation

1. **Level-Triggered, Not Edge-Triggered**: The operator doesn't just react to changes; it continuously ensures the actual state matches desired state
2. **Idempotent**: Running reconciliation multiple times has the same effect as running it once
3. **Eventually Consistent**: The system converges to the desired state over time
4. **Self-Healing**: If someone manually deletes a Deployment, the operator recreates it

## Code Walkthrough

### The Reconcile Function

The heart of any operator is the `Reconcile()` function. Here's what happens in our implementation:

```go
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // STEP 1: Fetch the WebApp resource
    webapp := &WebApp{}
    err := r.Get(ctx, req.NamespacedName, webapp)

    // STEP 2: Check if Deployment exists, create if not
    deployment := &appsv1.Deployment{}
    err = r.Get(ctx, types.NamespacedName{Name: webapp.Name, Namespace: webapp.Namespace}, deployment)
    if errors.IsNotFound(err) {
        // Create deployment
        dep := r.deploymentForWebApp(webapp)
        err = r.Create(ctx, dep)
    }

    // STEP 3: Reconcile drift - ensure Deployment matches spec
    if *deployment.Spec.Replicas != webapp.Spec.Replicas {
        deployment.Spec.Replicas = &webapp.Spec.Replicas
        err = r.Update(ctx, deployment)
    }

    // STEP 4: Create Service if needed
    // ... similar logic

    // STEP 5: Update status
    webapp.Status.AvailableReplicas = deployment.Status.AvailableReplicas
    err = r.Status().Update(ctx, webapp)

    // STEP 6: Requeue to check again later
    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
```

### Owner References

Notice the `SetControllerReference()` calls:

```go
controllerutil.SetControllerReference(webapp, dep, r.Scheme)
```

This creates an owner reference, meaning:
- The Deployment is "owned" by the WebApp
- If the WebApp is deleted, Kubernetes automatically deletes the Deployment (garbage collection)
- This establishes the parent-child relationship

### Watching Related Resources

```go
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&WebApp{}).              // Primary resource to watch
        Owns(&appsv1.Deployment{}). // Trigger reconciliation when owned Deployments change
        Owns(&corev1.Service{}).    // Trigger reconciliation when owned Services change
        Complete(r)
}
```

This tells the operator to trigger reconciliation when:
- A WebApp resource changes
- A Deployment owned by a WebApp changes
- A Service owned by a WebApp changes

## Project Structure

```
operator/
├── crd/
│   └── webapp-crd.yaml          # Custom Resource Definition
├── examples/
│   ├── nginx-webapp.yaml        # Example CR: nginx application
│   └── simple-webapp.yaml       # Example CR: hello-app
├── main.go                      # Operator implementation
├── go.mod                       # Go dependencies
├── Dockerfile                   # Container image for operator
├── Makefile                     # Convenience commands
└── README.md                    # This file
```

## Running the Operator

### Prerequisites

- Kubernetes cluster (minikube, kind, or any cluster)
- kubectl configured
- Go 1.21+ (for local development)

### Install the CRD

```bash
kubectl apply -f crd/webapp-crd.yaml
```

Verify:
```bash
kubectl get crd webapps.example.com
```

### Run the Operator Locally

```bash
go mod download
go run main.go
```

The operator will connect to your Kubernetes cluster and start watching for WebApp resources.

### Create a WebApp Resource

In another terminal:

```bash
kubectl apply -f examples/nginx-webapp.yaml
```

### Watch the Reconciliation

```bash
# Watch the WebApp status
kubectl get webapps -w

# Check what the operator created
kubectl get deployments
kubectl get services
kubectl get pods
```

### Test the Reconciliation Loop

Try these experiments to see reconciliation in action:

```bash
# 1. Scale the replicas
kubectl patch webapp nginx-app -p '{"spec":{"replicas":5}}' --type=merge

# 2. Manually delete the deployment (operator will recreate it)
kubectl delete deployment nginx-app

# 3. Check the status
kubectl describe webapp nginx-app
```

## Why Use Operators?

### 1. Benefits of Operators

- **Automation**: Codify operational knowledge (backups, upgrades, scaling)
- **Consistency**: Same operational practices across environments
- **Kubernetes-Native**: Leverage existing Kubernetes primitives
- **Declarative**: Users specify "what" they want, operator figures out "how"

### 2. Real-World Examples

- **Prometheus Operator**: Manages Prometheus monitoring deployments
- **Strimzi**: Manages Apache Kafka clusters
- **MySQL Operator**: Handles database provisioning, backups, failover
- **Cert-Manager**: Automates TLS certificate management

### 3. When to Build an Operator

Build an operator when:
- Application requires complex operational knowledge
- Need to automate Day 2 operations (upgrades, backups, recovery)
- Managing stateful applications
- Need to extend Kubernetes with domain-specific abstractions

### 4. Operator Frameworks

- **Operator SDK**: Full framework with scaffolding tools (Go, Ansible, Helm)
- **KubeBuilder**: Go-focused framework, used by Operator SDK under the hood
- **controller-runtime**: Lower-level library (what we used in this example)

### 5. Operator Maturity Levels

1. **Basic Install**: Automated application provisioning
2. **Seamless Upgrades**: Patch and minor version upgrades
3. **Full Lifecycle**: App lifecycle, storage, networking
4. **Deep Insights**: Metrics, alerts, log processing
5. **Auto Pilot**: Horizontal/vertical scaling, auto-config tuning, abnormality detection

Our example is at level 1-2.

## Advanced Topics to Mention

### Webhooks

Operators can include validating and mutating webhooks:
- **Validating**: Reject invalid WebApp configurations before they're stored
- **Mutating**: Set default values automatically

### Leader Election

When running multiple operator replicas:
- Only one actively reconciles (the leader)
- Others wait to take over if leader fails
- Ensures only one operator makes changes at a time

### Finalizers

Allow operators to perform cleanup before resource deletion:
```go
if webapp.ObjectMeta.DeletionTimestamp.IsZero() {
    // Add finalizer
    controllerutil.AddFinalizer(webapp, "example.com/finalizer")
} else {
    // Resource being deleted, perform cleanup
    if controllerutil.ContainsFinalizer(webapp, "example.com/finalizer") {
        // Cleanup logic here (e.g., delete external resources)
        controllerutil.RemoveFinalizer(webapp, "example.com/finalizer")
    }
}
```

### Status Conditions

We use Kubernetes-standard conditions to report state:
```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: DeploymentReady
    message: "Deployment has 3/3 replicas available"
    lastTransitionTime: "2024-01-18T10:30:00Z"
```

## Frequently Asked Questions

**Q: What's the difference between an operator and a controller?**
A: All operators are controllers, but not all controllers are operators. Operators are controllers that use CRDs to manage application-specific resources with domain knowledge.

**Q: What happens if the operator crashes?**
A: When it restarts, it will reconcile all existing WebApp resources and bring them to the desired state. The reconciliation loop is level-triggered, not edge-triggered.

**Q: How do you handle concurrent updates?**
A: Kubernetes uses optimistic locking with resource versions. If two updates conflict, one will fail with a conflict error and should retry.

**Q: How do you test operators?**
A: Use envtest (runs a local control plane), unit tests for reconciliation logic, and end-to-end tests in real clusters.

**Q: What about performance with many resources?**
A: Use caching, selective watching (predicates), and rate limiting. controller-runtime includes a built-in cache.

**Q: How do you handle upgrades of the CRD itself?**
A: CRDs support versioning. You can have multiple versions (v1alpha1, v1beta1, v1) simultaneously. Use conversion webhooks to translate between versions. The stored version is what's persisted in etcd.

## Clean Up

```bash
# Delete example resources
kubectl delete -f examples/

# Delete CRD (this also deletes all WebApp resources)
kubectl delete -f crd/webapp-crd.yaml
```

## Further Learning

- **Operator SDK**: https://sdk.operatorframework.io/
- **KubeBuilder Book**: https://book.kubebuilder.io/
- **controller-runtime**: https://github.com/kubernetes-sigs/controller-runtime
- **Operator Hub**: https://operatorhub.io/ (browse existing operators)

## Summary

This example demonstrates:
- ✅ Custom Resource Definition (CRD) to extend Kubernetes
- ✅ Reconciliation loop pattern for continuous state management
- ✅ Creating and managing child resources (Deployment, Service)
- ✅ Status reporting and conditions
- ✅ Owner references for garbage collection
- ✅ Real-world operator pattern using controller-runtime

The operator pattern is powerful because it allows you to encode operational expertise into software that runs continuously, ensuring your applications stay healthy and properly configured without manual intervention.
