# Operator Architecture

## Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      Kubernetes API Server                  │
│                                                             │
│  ┌─────────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ WebApp CRD      │  │ Deployments  │  │ Services     │  │
│  │ (Custom)        │  │ (Built-in)   │  │ (Built-in)   │  │
│  └─────────────────┘  └──────────────┘  └──────────────┘  │
└──────────────┬──────────────────┬───────────────┬──────────┘
               │                  │               │
               │ Watch            │ Watch         │ Watch
               │                  │               │
         ┌─────▼──────────────────▼───────────────▼─────┐
         │         WebApp Operator (Controller)          │
         │                                               │
         │  ┌──────────────────────────────────────┐    │
         │  │  Manager                             │    │
         │  │  - Starts/stops controllers          │    │
         │  │  - Manages shared cache              │    │
         │  │  - Handles leader election           │    │
         │  └──────────────────────────────────────┘    │
         │                                               │
         │  ┌──────────────────────────────────────┐    │
         │  │  Informer (Cache)                    │    │
         │  │  - Watches API server                │    │
         │  │  - Local cache of resources          │    │
         │  │  - Reduces API server load           │    │
         │  └──────────────────────────────────────┘    │
         │                                               │
         │  ┌──────────────────────────────────────┐    │
         │  │  Work Queue                          │    │
         │  │  - Receives watch events             │    │
         │  │  - Deduplicates requests             │    │
         │  │  - Rate limiting                     │    │
         │  └──────────────────────────────────────┘    │
         │                                               │
         │  ┌──────────────────────────────────────┐    │
         │  │  WebAppReconciler                    │    │
         │  │  - Reconcile() function              │    │
         │  │  - Business logic                    │    │
         │  │  - Creates/updates resources         │    │
         │  └──────────────────────────────────────┘    │
         └───────────────────────────────────────────────┘
```

## Reconciliation Flow

```
User Action: kubectl apply -f webapp.yaml
                      │
                      ▼
          ┌───────────────────────┐
          │  API Server           │
          │  - Validates          │
          │  - Stores in etcd     │
          └───────────┬───────────┘
                      │
                      │ Watch Event
                      ▼
          ┌───────────────────────┐
          │  Informer (Cache)     │
          │  - Receives event     │
          │  - Updates cache      │
          └───────────┬───────────┘
                      │
                      │ Enqueue
                      ▼
          ┌───────────────────────┐
          │  Work Queue           │
          │  - Stores req         │
          │  - Deduplicates       │
          └───────────┬───────────┘
                      │
                      │ Worker pulls
                      ▼
          ┌───────────────────────────────────────┐
          │  Reconcile(ctx, Request)              │
          │                                       │
          │  1. Fetch WebApp from cache          │
          │     ↓                                 │
          │  2. Check Deployment exists          │
          │     ├─ No  → Create Deployment       │
          │     └─ Yes → Check if matches spec   │
          │                ├─ No  → Update it    │
          │                └─ Yes → Continue     │
          │     ↓                                 │
          │  3. Check Service exists             │
          │     ├─ No  → Create Service          │
          │     └─ Yes → Continue                │
          │     ↓                                 │
          │  4. Update WebApp status             │
          │     - availableReplicas              │
          │     - conditions                     │
          │     ↓                                 │
          │  5. Return Result                    │
          │     - RequeueAfter: 30s              │
          └───────────┬───────────────────────────┘
                      │
                      │ Success
                      ▼
          ┌───────────────────────┐
          │  Wait 30s             │
          │  Then reconcile again │
          └───────────────────────┘
```

## Resource Relationships

```
┌────────────────────────────────────────────┐
│  WebApp CR                                 │
│  apiVersion: example.com/v1                │
│  kind: WebApp                              │
│  metadata:                                 │
│    name: nginx-app                         │
│  spec:                                     │
│    image: nginx:1.25                       │
│    replicas: 3                             │
└────────────┬───────────────────────────────┘
             │
             │ Owner Reference
             │ (Parent-Child)
             │
    ┌────────┴────────┐
    │                 │
    ▼                 ▼
┌───────────────┐  ┌──────────────┐
│  Deployment   │  │  Service     │
│  nginx-app    │  │  nginx-app   │
│  replicas: 3  │  │  port: 80    │
└───────┬───────┘  └──────────────┘
        │
        │ Owns
        │
        ▼
┌───────────────────┐
│  ReplicaSet       │
│  nginx-app-xyz123 │
└───────┬───────────┘
        │
        │ Owns
        │
        ▼
┌───────────────────┐
│  Pods (3)         │
│  - nginx-app-abc  │
│  - nginx-app-def  │
│  - nginx-app-ghi  │
└───────────────────┘
```

## Watch Mechanism

The operator watches three types of resources:

```
┌─────────────────────────────────────────────────┐
│  SetupWithManager()                             │
│                                                 │
│  For(&WebApp{})                                 │
│    ↓                                            │
│    Primary watch - triggers reconcile when:    │
│    - WebApp created                            │
│    - WebApp updated                            │
│    - WebApp deleted                            │
│                                                 │
│  Owns(&Deployment{})                           │
│    ↓                                            │
│    Secondary watch - triggers reconcile when:  │
│    - Owned Deployment changes                  │
│    - Owned Deployment deleted                  │
│                                                 │
│  Owns(&Service{})                              │
│    ↓                                            │
│    Secondary watch - triggers reconcile when:  │
│    - Owned Service changes                     │
│    - Owned Service deleted                     │
└─────────────────────────────────────────────────┘
```

## State Machine

```
WebApp Lifecycle States:

┌─────────────┐
│   Created   │  User creates WebApp CR
└──────┬──────┘
       │
       ▼
┌─────────────────────┐
│   Reconciling       │  Operator creates Deployment & Service
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│   Waiting for       │  Deployment rolling out
│   Pods Ready        │
└──────┬──────────────┘
       │
       ▼
┌─────────────────────┐
│   Ready             │  All replicas available
│   (Normal State)    │
└──────┬──────────────┘
       │
       ├──────────────────┐
       │                  │
       │ User scales      │ User deletes
       │                  │
       ▼                  ▼
┌─────────────────┐  ┌─────────────────┐
│   Scaling       │  │   Deleting      │
└──────┬──────────┘  └──────┬──────────┘
       │                     │
       │                     ▼
       │              ┌──────────────┐
       │              │   Deleted    │
       │              └──────────────┘
       │
       └──────► (back to Ready)
```

## Error Handling & Retry

```
┌──────────────────────────────────────┐
│  Reconcile() returns error           │
└────────────┬─────────────────────────┘
             │
             ▼
┌──────────────────────────────────────┐
│  controller-runtime error handler    │
│  - Logs error                        │
│  - Increments retry counter          │
└────────────┬─────────────────────────┘
             │
             ▼
┌──────────────────────────────────────┐
│  Rate limiter calculates backoff     │
│  - First retry: ~1s                  │
│  - Second retry: ~2s                 │
│  - Third retry: ~4s                  │
│  - ...                               │
│  - Max: ~5 minutes                   │
└────────────┬─────────────────────────┘
             │
             ▼
┌──────────────────────────────────────┐
│  Request re-added to work queue      │
│  after backoff period                │
└────────────┬─────────────────────────┘
             │
             └──► Reconcile() called again
```

## Production Considerations

### High Availability

```
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│  Operator    │  │  Operator    │  │  Operator    │
│  Replica 1   │  │  Replica 2   │  │  Replica 3   │
│  (Leader)    │  │  (Standby)   │  │  (Standby)   │
└──────┬───────┘  └──────┬───────┘  └──────┬───────┘
       │                 │                 │
       │ Lease acquired  │ Waiting         │ Waiting
       │                 │                 │
       └─────────────────┴─────────────────┘
                         │
                         ▼
              ┌────────────────────┐
              │  Leader Election   │
              │  (Lease in K8s)    │
              └────────────────────┘
```

### Metrics & Observability

```
Operator exposes metrics:
- reconcile_duration_seconds
- reconcile_errors_total
- reconcile_total
- workqueue_depth

↓ Scraped by

Prometheus → Grafana Dashboards
            → Alerts (PagerDuty, etc.)
```

### Webhooks (Advanced)

```
kubectl create -f webapp.yaml
         │
         ▼
┌─────────────────────┐
│  API Server         │
└─────────┬───────────┘
          │
          │ Call webhook
          ▼
┌─────────────────────┐
│  Validating Webhook │ ← Running in operator
│  - Check image tag  │
│  - Validate replicas│
└─────────┬───────────┘
          │
          │ Allow/Deny
          ▼
┌─────────────────────┐
│  Mutating Webhook   │ ← Running in operator
│  - Set defaults     │
│  - Add labels       │
└─────────┬───────────┘
          │
          │ Modified spec
          ▼
┌─────────────────────┐
│  Store in etcd      │
└─────────────────────┘
```

## Key Design Patterns

### 1. Level-Triggered (Not Edge-Triggered)
- Operator doesn't just react to events
- Periodically checks and reconciles state
- Self-healing: detects manual changes

### 2. Declarative API
- User declares desired state
- Operator figures out how to achieve it
- No imperative commands

### 3. Controller Pattern
- Watch → Compare → Act loop
- Eventual consistency
- Idempotent operations

### 4. Owner References
- Garbage collection
- Cascading deletes
- Resource relationships

### 5. Status Subresource
- Separate from spec
- Reflects observed state
- Standard conditions
