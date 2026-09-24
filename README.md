# Docker & Kubernetes Concepts

## Purpose

A reference repo: every Dockerfile, Compose file and Kubernetes manifest
here exists to demonstrate one or more Docker/Kubernetes concepts in
context, with comments explaining the *why* next to the thing being
demonstrated. See the two tables below for exactly what's demonstrated
where, and what's deliberately left out.

Two services are real, working code, wired end-to-end through Docker
Compose and Kubernetes alike: **items-api** (`backend/` — a minimal
Spring Boot service, one endpoint: `GET /api/hello`) and **items-frontend**
(`frontend/` — a minimal Angular page: a button that calls it). Both live
at the repo root, one level up from the Docker examples in `docker/` --
they're real application projects, not just a Dockerfile demonstrating
concepts. Click the button, get "Hello from Backend" back — see "Running
things locally" below for the exact tested steps, both via `docker compose`
and via a real kind/minikube cluster.

`docker/worker` is real too: a small Go binary (stdlib only) that logs the
container's load average and available memory on an interval, reading
`/proc` directly. It builds and runs as an always-on service in
`docker/docker-compose.yml`, alongside backend and frontend.

`onbuild-base`/`onbuild-consumer` and `add-vs-copy` stay illustrative-only
on purpose: those reference app source (`package.json`, `app.js`,
`release.tar.gz`, ...) that doesn't exist in this repo, so building them
fails past their first `COPY`/`ADD` step. That's expected there — the point
is to read the Dockerfile and its comments, not to end up with a working
app.

The Kubernetes side is small but real end to end: the `db-migration` Job
creates a real `items` table in the real Postgres, the `nightly-cleanup`
CronJob deletes rows older than 30 days from it, and the `worker`
DaemonSet is a tiny per-node agent that logs the node's load and free
memory.

## Theoretical Background

### Docker -- container technology

Docker runs containers on **one** machine. It has no built-in self-healing
(a crashed container stays crashed until something restarts it), no
load balancing across machines, and no automatic scaling -- those are
exactly the gaps Kubernetes exists to fill.

#### Core objects

```
Dockerfile -> Image -> Container
              ^         |
           Registry   Volume / Network (attached at runtime)

Dockerfile -- build instructions (declarative recipe).
Image      -- immutable, layered filesystem snapshot built from a Dockerfile. Never runs by itself.
Layer      -- each Dockerfile instruction (RUN, COPY, etc.) adds a read-only layer; layers are cached and shared across images with the same base.
Container  -- a running (or stopped) instance of an image: the image's layers + one thin writable layer on top + an isolated process (namespaces + cgroups).
Registry   -- where images are stored/versioned (Docker Hub, ECR, GHCR). push/pull move images to/from it.
Volume     -- persistent storage that outlives a container's writable layer; lives outside the container's lifecycle.
Network    -- how containers reach each other and the outside world (bridge, host, none, overlay in Swarm).

Container ≠ VM: no separate kernel, so isolation is namespaces/cgroups, not virtualization -- faster startup, weaker isolation boundary than a VM.
Root by default: containers run as root inside the container unless the Dockerfile sets USER.
Ephemeral by default: anything written inside a container's writable layer is gone on docker rm unless it's in a volume -- the #1 source of "my data disappeared" confusion.
```

#### Build flow

```
Layers:
            Each instruction (FROM, COPY, ENV, CMD..) = new layer;
Caching:
            Unchanged layers (instructions) are cache-reused on rebuild (instruction order matters — put rarely-changing steps before COPY ./ ./).
            Invalid cache layer also invalidate all succeeding layers.
Multi-staging builds:
            Every FROM is one separate stage, and only the final stage(FROM) becomes the image, this is reason why last stage should always be as small as possible.
            Keeps build toolchain (Maven, npm) out of the final runtime image.
Profiling:
            - A service-gating mechanism, not a merge mechanism.
            Can only decide whether a whole service block starts or not (inside compose.yaml file).
            Cannot edit or add fields to a service conditionally.
            Can deactivate 'base' service.
Composing:
            - A file-merge mechanism, structurally like Kubernetes's Kustomize overlays.
            Can edit fields of an existing service, add new fields to an existing service, add whole new services.
            Cannot delete a service the base defines — that's the gap to the Kustomize overlays.
```

#### Run flow

```
Registry --docker pull--> Image --docker run--> Container (writable layer + namespaces + cgroups)
    docker run = implicit pull-if-missing + create + start.
Container gets its own namespaces:
    PID (private proccess tree)
    net(private IP + port)
    mount(private filesystem)
        writable layer (default, container-bound, overlay2 diff) is ephemeral — deleted with the container (survives start/stop).
        volume/mount (persists independently) - survives container removal

PID 1 signal handling: the process you CMD/ENTRYPOINT runs as PID 1 inside the container.
    'tini' as PID 1: forwards signals (shell as PID 1 often doesn't forward SIGTERM to its child) and reaps zombies (also term/kill detached/forgoten subprocesses).
```

#### Networking flow

```
Container A --'user-defined bridge' network (DNS by container name)--> Container B
    - containers can use each other's 'name' instead of their private IPs (easier addressing)
Host <--port mapping (-p 8080:80)-- Container (published port)
    - external clients must use 'host IP/localhost' + 'defined container port'

Docker's 'default bridge' has no DNS between containers (must link each other by IP), but a 'user-defined bridge' network gives you name-based service(containers) discovery.
    - Compose always creates 'user-defined bridge' automatically per project (even 'default' when no network: block and name provided).
```

#### Compose flow (multi-container on one host)

```
docker-compose.yml --docker compose up--> builds/pulls images, creates network, starts all services, wires DNS between them
    - One command replaces manually running docker build + docker network create + several docker runs with consistent flags.

Environments:
    env_file: - auto parsed KEY:VALUE pair from file
    environment: - hardcoded strings or variables defined within this Compose file (cannot easly parse from file)

Secrets:
    File-based secret: ./secrets/api_token.txt is exposed only under /run/secrets/ in services that list it, never as an env var.
    Env-based secret: value taken from an environment variable (e.g., `export API_TOKEN_VALUE=...` before docker compose), instead of a file.
    External: references a secret created outside this file (`docker secret create ...`) by name, instead of Compose creating one.
    Driver-based: value fetched live from an external store (Vault, AWS Secrets Manager, ...) via a plugin, instead of a static file/env var.

Configs: like `secrets:`, but for non-sensitive files.
```

### Kubernetes -- coordinator (manager)

Kubernetes runs containers across a **cluster** of machines, and actively
keeps the running state matching what you declared -- restarting crashed
Pods, load-balancing across however many currently exist, and (with a
HorizontalPodAutoscaler, HPA) scaling that count with real demand.

#### Core objects

```
Cluster / Node (VM) / Pod (scheduling unit) / Container (running process) -- where things physically live.

Controller (Deployment / StatefulSet / DaemonSet / Job) -- creates/deletes pods to keep them alive.
  Deployment  -> Pods are interchangeable                    (backend-deployment.yaml)
  StatefulSet -> Pods keep stable identity + their own disk  (database-statefulset.yaml)
  DaemonSet   -> one Pod per node                             (worker-daemonset.yaml)
  Job/CronJob -> Pod(s) that run once/on schedule, not forever

Ingress -> Service -> Pods -- how traffic finds Pods.
  Ingress       -> external entry point, L7 host/path routing                          (networking/ingress.yaml)
  Service       -> stable in-cluster DNS name + load-balances across matching Pods      (networking/backend-service.yaml, networking/database-service.yaml, ...)
  NetworkPolicy -> firewall rule for Pods -- who's *allowed* to reach them              (networking/networkpolicy.yaml)

Identity & Config -- what a Pod authenticates as, and what config/secrets it's handed.
  ServiceAccount        -> the identity a Pod's containers use against the Kubernetes API   (config/serviceaccount.yaml)
  Role / RoleBinding     -> what that ServiceAccount is actually permitted to do             (config/role-based-access-control.yaml)
  ConfigMap              -> non-secret config injected into a Pod, via env or volume         (config/configmap.yaml)
  Secret                 -> same idea, for credentials, kept separate                        (config/secret.yaml)

Policies -- cluster-wide policy applied across every Pod in a namespace, not to any one Pod.
  Namespace              -> the isolation boundary everything above lives inside             (namespace.yaml)
  ResourceQuota           -> caps total resource consumption across the whole namespace       (policy/resourcequota.yaml)
  LimitRange               -> default/cap on a single container's resources                    (policy/limitrange.yaml)
  HorizontalPodAutoscaler   -> adjusts a Controller's replica count based on live metrics       (policy/horizontal-pod-autoscaler.yaml)
  PodDisruptionBudget        -> caps how many Pods a voluntary disruption can take down at once (policy/pod-disruption-budget.yaml)
```

#### Common controllers use-cases

```
- Deployment: stateless, always-on services whose Pods are interchangeable, such as web frontends, REST APIs and queue consumers. Scale by changing the replica count.
- StatefulSet: workloads where each Pod needs a stable name and its own disk, such as databases (Postgres, MySQL), message brokers (Kafka, RabbitMQ) and clustered stores (etcd, Elasticsearch).
- DaemonSet: node-level agents that must run on every node, such as log shippers (Fluent Bit), node metrics collectors (node-exporter), network and storage plugins, and security agents.
- Job: one-off tasks that run to completion once, such as database schema migrations, data imports and backfills, batch processing and one-time setup.
- CronJob: recurring, finite tasks on a clock schedule, such as nightly cleanups, database backups, report generation and periodic data syncs.
```

#### Docker Desktop's - Kubernetes support Environment

```
Docker Desktop's provides Kubernetes support.
Docker Desktop's Kubernetes shares the local image cache, so no registry push is needed.

Docker Desktop's Kubernetes sub-system runs under a completely isolated container environment (containerd, driven by kubelet) from the classic Docker Engine (dockerd: docker ps/docker images).
As a result, you can see Docker images referenced by Kubernetes Deployments, but cannot see Docker containers defined by them.

Shared resources (pre-existing infrastructure):
    cluster (docker-desktop) and node(desktop-control-plane) - both are Docker Desktop's own default names, assigned automatically by Docker Desktop's -> Enable Kubernetes
    ingress-nginx - Ingress controller that makes Ingres object (routing rules) do anything
        - totally different purpose from frontend's nginx (only serves Angular static files)
    kube-system - namespace Kubernetes reserves for its own components
        kube-apiserver — The API server, the central record everything talks to
        etcd — The database behind the API server
        kube-scheduler — Picks a node for each new Pod
        kube-controller-manager — Runs the built-in controllers (Deployment, Job, and so on)
        kube-proxy — Implements Service routing on the node
        coredns (2 Pods) — In-cluster DNS. It is why backend and database resolve as names.
        kindnet — The network plugin that gives Pods their IPs

Why we'd potentially need more nodes:
    Raw capacity. One machine has finite CPU/RAM -- past that, pods don't fail, they just sit Pending forever with nowhere to fit.
    Fault tolerance. With one node, if it goes down (crash, reboot, a cloud provider doing maintenance), everything goes down -- every pod that was on it. Multiple nodes let Kubernetes spread replicas so losing one node only takes out what was on it.
    Rolling node maintenance without downtime. With multiple nodes, you can kubectl drain one at a time (evict its pods safely elsewhere, patch/reboot it, bring it back) while the others keep serving traffic. With one node, any maintenance on it means total downtime.
    Specialized hardware. Some workloads need something specific -- a GPU for ML, more local disk -- so you add nodes with that hardware and taint them so only workloads requesting it land there.

Why we need namespaces:
    A cluster relying only on its default namespace doesn't allow multiple objects to share the same name.
    Also, RBAC, ConfigMap, Secrets, etc. are namespace-scoped, so it's more natural for every object to have its own namespace defined.
```

#### Network and Routing

```
Routing without Ingress setup:
'Dev' overlay's (profile-k8s/overlays/dev/frontend-localhost.yaml) zero-setup http://localhost:4300 route:

Browser: http://localhost:4300
        |
        v
1. Windows loopback           127.0.0.1:4300 / [::1]:4300
        |
        v
2. WSL2 (wslrelay.exe)        [Linux VM running Docker's and Kubernetes' processes]
                               listens on :4300, mirrors straight into the VM at the SAME port :4300
                               (no translation here -- proven: netstat showed wslrelay.exe holding 127.0.0.1:4300)
        |
        v
3. Docker Desktop's Kubernetes LoadBalancer publisher ['cloud-provider-kind' service, inside the VM]
                               receives on :4300, forwards to the Kubernetes Service's ClusterIP, still at :4300
                               (no translation here either)
        |
        v
4. Kubernetes frontend Service [frontend-localhost.yaml -> Service "frontend-dev-localhost"]
                               -- TRANSLATION HAPPENS HERE --
                               port: 4300 -> targetPort: 4200
        |
        v
5. Endpoint (real Pod's IP:port, from `kubectl get endpoints`):
                               port: 4200
        |
        v
6. Container's nginx process  [frontend/Dockerfile]
                               listens on :4200 (baked in: frontend/nginx.conf -> `listen 4200;`)
        |
        v
7. nginx serves the compiled Angular static files (index.html, main-*.js, styles-*.css)
                               -> SAME port: 4200. There is no separate "Angular app port" --
                               Angular's own runtime never exists past build time (see
                               frontend/Dockerfile's two-stage build); nginx-on-4200 IS how
                               the app gets served.

Routing with Ingress setup:
The real Ingress route (docker-kubernetes-app.local, step 3 of "Local Preparation" below) inserts one more hop at steps 3-4 instead: host port 80/443 -> ingress-nginx-controller's own LoadBalancer Service and pod -> the Ingress object's routing rules -> the base frontend Service (ClusterIP, port 80, k8s/base/networking/frontend-service.yaml) -> the same pod, same nginx, same port 4200 from there on. Only one thing ever changes between the two routes: which Service you enter through.

        (same as steps 1-3 above, but on host port 80/443 instead of 4300)
        |
        v
4. ingress-nginx-controller pod [k8s/base/networking/ingress.yaml]
                               reads the Ingress object's host/path rules --
                               THIS is where the Ingress process itself sits
                               routes "/" -> the base frontend Service
        |
        v
5. Kubernetes frontend Service [frontend-service.yaml -> Service "frontend"]
                               -- TRANSLATION HAPPENS HERE --
                               port: 80 -> targetPort: 4200
        |
        v
   (from here on, identical to the Without-Ingress path: Endpoint -> nginx:4200 -> Angular static files)
```

#### Storage

```
There are three different roles (objects) in one pipeline, and normally all three exist together for a single volume:

PersistentVolumeClaim (PVC) -- a request for storage, made per-Pod. Doesn't provision anything by itself.
PersistentVolume (PV)       -- the actual disk that fulfills a PVC, either:
                                  dynamically provisioned (a StorageClass names the CSI driver that creates one to match), or
                                  a pre-created PV that already satisfies the PVC's size/access-mode.
StorageClass                -- names which CSI driver/provisioner to use when dynamically creating a PV.

For stateful Pods specifically (StatefulSet's volumeClaimTemplates):
  - Each replica gets its OWN PVC (data-database-0, data-database-1, ...), never one shared PVC.
  - A Pod always rebinds to that SAME PVC, even after being rescheduled to a different node.
  - That 1:1, stable Pod<->PVC link is exactly what "stable identity" buys over a Deployment,
    where Pods are interchangeable and share no such storage relationship at all.
```

#### Security

```
RBAC - Role Based Access Control

Flow:
    Deployment --serviceAccountName  -->  ServiceAccount  <--  subjects-- RoleBinding --roleRef  -->  Role --rules  -->  ConfigMap, Secret

Rules (Kubernetes API; object-level):
    resource's kind + name of defined object + verb

Verbs:
    get, list, watch, create, update, patch, delete, deletecollection, *
```

#### Overlays

```
Base - k8s/base/kustomization.yaml
    - shared necessarily Kubernetes objects of project
Overlays/Profiles(dev, prod) - k8s/[dev|prod]/kustomization.yaml
    - kustomization.yaml/patches:
        edit fields of existing 'base' objects (use same 'base' metadata/name)
        remove 'base' objects (start with $patch: delete and same 'base' metadata/name)
    - kustomization.yaml/resources:
        add new objects (alongside ../../base)
```

#### Workload

```
kubectl apply -k  -->  kube-apiserver (validates)  -->  etcd (recorded -- nothing running yet)
                                                              |
                                                              v
                                            A controller creates the child objects
                     Deployment -> ReplicaSet -> Pods      StatefulSet -> Pods (stable identity)
                     DaemonSet -> one Pod per node          CronJob -> Job -> Pods (on schedule)
                                                              |
                                                              v
                                     kube-scheduler picks a node for each new Pod
                                                              |
                                                              v
                                  kubelet (on that node) pulls the image, starts containers
                                                              |
                                                              v
                                  Status flows back up: kubelet -> apiserver -> etcd
                                       (what `kubectl get`/`describe` are reading)
                                                              |
                                                              v
                        Loops forever -- HPA/PDB watch and adjust on top of this, never a one-shot

- Applying just records desired state -- nothing runs yet.
- A controller turns that into real child objects (Pods).
- The scheduler decides where -- the kubelet decides how it actually starts.
- Everything reports status back the same way it came in, and the loop never stops.
- HorizontalPodAutoscaler (auto-scaler) and PodDisruptionBudget sit on top of this same loop:
    HPA adjust replicas count to serve requests, PDB preserve minimal needed replicas count to request serving works during disruptions.
```

### Docker Compose vs Kubernetes

Kubernetes never runs Compose, and Compose never runs Kubernetes. They are two separate ways to run the same images.

Both start from the same Dockerfiles (`backend/`, `frontend/`), but each has its own definition files (`docker/docker-compose.yml` vs `k8s/`) and its own container runtime.
Compose is a single-host tool with no API server and no controllers like Kubernetes.
This repo implements the same app both ways, so most Compose features have a Kubernetes counterpart: the `db-migration` service ↔ Job, `db-net` ↔ NetworkPolicy, `x-hardened` ↔ securityContext.

### Difference: time-schedule vs node-placement scheduling (Kubernetes)

Two unrelated things both get called "scheduling" in Kubernetes, which is
an easy source of confusion:

- **Node-placement scheduling:** which node it runs on, decided using
  resource requests, `affinity`/`podAntiAffinity`, `topologySpreadConstraints`,
  `tolerations`. Nothing to do with time.
- **Time-schedule:** when a Job gets created, decided by the CronJob
  controller watching the clock. Nothing to do with nodes.

Every Deployment's Pods go through node-placement scheduling only; a
CronJob's Pods go through *both* -- the CronJob controller decides *when*
to create the underlying Job (time-schedule), and `kube-scheduler` still
separately decides *where* that Job's Pod lands (node-placement
scheduling), exactly like any other Pod.

### Orchestration vs. Choreography (Kubernetes)

- **Orchestration** is a centralized, command-driven coordination pattern
  -- a central controller tells individual services exactly what to do
  and when to do it.
- **Choreography** is a decentralized, event-driven coordination pattern
  -- individual services act autonomously, broadcasting and reacting to
  events, with no central conductor.

Kubernetes is a genuine hybrid of the two, which is worth knowing rather
than filing it under just "Kubernetes is a container orchestrator":

- At the **cluster** level it looks like Orchestration: the API server is
  one central source of defined objects (`kubectl apply`), but it doesn't
  tell individual services exactly what to do (only Orchestrator-like).
- At the **controller** level it actually works like Choreography: there
  is no central process telling the Deployment controller, the scheduler
  and the kubelet what to do, in order.  
  Each runs its own independent
  reconciliation loop, watching the API server and reacting autonomously (Choreography-style)
  -- a Deployment controller doesn't *command* a kubelet to start a
  container, it just writes a Pod object, and the kubelet on the right
  node notices it exists and reacts on its own.

So: Kubernetes centralizes *desired state* (Orchestration's trait) but
decentralizes *how that state gets achieved* (Choreography's trait) --
part of why it scales better than a single central controller issuing
every command directly would.

## Layout

The two real projects live at the repo root; `docker/worker` is real too
(see "Purpose" above); only `onbuild-base`/`onbuild-consumer` and
`add-vs-copy` stay illustrative-only, under `docker/`. Everything
Kubernetes-related is under `k8s/`:

```
backend/                 items-api -- real Spring Boot project (Dockerfile, pom.xml, src/)
frontend/                items-frontend -- real Angular project (Dockerfile, package.json, angular.json, src/, nginx.conf)
                         nginx server is 2nd stage of Dockerfile to serve static files

docker/
├── worker/                  Real Go binary (stdlib only, logs load/memory from /proc)
├── onbuild-base/            Base image using ONBUILD
├── onbuild-consumer/        Consumes onbuild-base -- FROM only, no other instructions
├── add-vs-copy/             ADD's two special behaviors (local archive extraction, remote URL fetch) vs plain COPY
├── secrets/                 api_token.txt -- the file docker-compose.yml's `secrets:` block reads
├── migrations/              create-items-table-script.sql -- schema the db-migration service runs, mounted via Compose `configs:`
├── .env                     NODE_VERSION/JAVA_VERSION -- compose-side build-arg interpolation
├── docker-compose.yml       Compose Specification
└── docker-compose.dev.yml   -f override on top of docker-compose.yml

k8s/
├── base/                  kustomization.yaml + namespace.yaml, plus one object family per file, packaged by role:
│   ├── controllers/       Deployment, StatefulSet, DaemonSet, Job, CronJob
│   ├── networking/        Ingress, Service, NetworkPolicy
│   ├── config/            ConfigMap, Secret, ServiceAccount, Role/RoleBinding
│   └── policy/            ResourceQuota, LimitRange, HorizontalPodAutoscaler, PodDisruptionBudget
└── overlays/
    ├── dev/                 1 replica, :dev image tags, LOG_LEVEL=debug patched on top of base
    └── prod/                3 replicas, pinned :prod image tags, base's LOG_LEVEL=info left alone
```

## k8s files purpose

Covers `k8s/base/` only -- `k8s/overlays/` isn't a separate set of objects,
it's only for environment profiling (`dev`/`prod`), patching values on top
of these same base files.

**Our single node sample includes:**

| File | Kind | What it does, and why it exists |
|---|---|---|
| `namespace.yaml` | Namespace | A cluster relying only on its `default` namespace doesn't allow multiple objects to share the same name — that's why we need to define different namespaces. Also, RBAC, ConfigMap, Secrets, etc. are namespace-scoped, so it's more natural for every object to have its own namespace defined. |
| `kustomization.yaml` | Kustomize config | Not a Kubernetes object — a tooling instruction listing every file below as one unit. Without it, `kubectl apply -f k8s/base/` wouldn't look inside the `controllers/`/`networking/`/`config/`/`policy/` subfolders at all and would silently deploy almost nothing. |
| **Controllers** | | It defines Pods (the scheduling unit) and their Containers (the actual running processes). |
| `controllers/backend-deployment.yaml` | Deployment | Runs self-healing copies of items-api (Spring Boot). Includes health checks, resource limits, and a locked-down (non-root) security context. The `replicas: 2` runs all pods at once, continuously enforced by self-healing; to change that count, edit this file manually or define it dynamically with `HorizontalPodAutoscaler` instead. |
| `controllers/frontend-deployment.yaml` | Deployment | Same self-healing guarantee, for items-frontend (the Angular page + button), served as static files through nginx instead of a JVM. |
| `controllers/database-statefulset.yaml` | StatefulSet | Runs Postgres. Not a Deployment on purpose: a Deployment treats replicas as interchangeable, but a database pod's disk isn't interchangeable — losing it means losing data. A StatefulSet gives the pod a fixed name (`database-0`) and its own disk that follows it across restarts. |
| `controllers/worker-daemonset.yaml` | DaemonSet | A tiny node-metrics agent (logs the node's load and free memory every 30s) that runs on *every* node, not a fixed count. Add a node, get one more copy automatically — no other controller type does that. |
| `controllers/migration-job.yaml` | Job | A one-time task that runs to completion and stops — here it creates the `items` table with `psql` (idempotent). Unlike a Deployment, it isn't restarted afterward, because it's not meant to run forever. |
| `controllers/cleanup-cronjob.yaml` | CronJob | The same idea as a Job, but on a repeating schedule (`0 2 * * *`, pinned to UTC with `timeZone`) — here it deletes `items` rows older than 30 days. A thin wrapper that creates a new Job automatically, the same way OS-level cron would. |
| **Networking** | | It defines access points to pods. |
| `networking/ingress.yaml` | Ingress ("router-like") | The one public front door, which declares the DNS hostname (`docker-kubernetes-app.local`) it routes for. Routes by path: `/api/*` → backend, everything else → frontend. Without it there'd be no single coherent URL for the whole app — because each Service would get its own separate IP address. |
| `networking/backend-service.yaml` | Service ("load-balancer-like") | It defines a stable virtual IP address and DNS hostname — so every pod behind it is reachable under that one same name/address. Without it, the caller must know individual pod IPs (which change on every restart). Internally load-balances requests across the existing pods pool (does not influence its size). |
| `networking/frontend-service.yaml` | Service | Same idea, for the frontend pods — the other half of what `ingress.yaml` routes to. |
| `networking/database-service.yaml` | Service (headless) | Deliberately *not* load-balanced (`clusterIP: None`) — gives each Postgres pod its own individually addressable DNS name, matching a StatefulSet's one-identity-per-pod model. |
| `networking/networkpolicy.yaml` | NetworkPolicy | A firewall scoped to pods, not machines. Backend may only receive traffic from the frontend and only send traffic to the database + DNS — even a compromised pod elsewhere can't reach it directly. |
| **Config** | | |
| `config/serviceaccount.yaml` | ServiceAccount | It defines named identities that deployments/pods can reference to be recognizable via the Kubernetes API (ex. `role-based-access-control.yaml`). `serviceaccount.yaml` defines it; `controllers/backend-deployment.yaml` only references it. |
| `config/configmap.yaml` | ConfigMap | Non-sensitive settings (e.g. log level) injected into the backend as env vars, kept outside the image so the same image can run with different config per environment without a rebuild. |
| `config/secret.yaml` | Secret | Same idea as ConfigMap, but for credentials — kept as a separate object type so viewing non-sensitive config doesn't also expose sensitive values. |
| `config/role-based-access-control.yaml` | Role + RoleBinding | Defines exactly what that identity may do (read ConfigMaps/Secrets, nothing else) — least privilege, so even if compromised, the blast radius is tiny. |
| **Policies** | | |
| `policy/resourcequota.yaml` | ResourceQuota | A hard ceiling on *total* CPU/memory/pod count across the whole namespace — a safety net above all single pod's own limits. |
| `policy/limitrange.yaml` | LimitRange | Fills in a default CPU/memory request+limit for any container that forgets to set its own. Every container here already does, so this mostly documents the pattern. |
| `policy/horizontal-pod-autoscaler.yaml` | HorizontalPodAutoscaler | Watches the backend's real CPU/memory and adjusts replica count automatically (2–6) — scale with actual demand instead of permanently paying for peak capacity. |
| `policy/pod-disruption-budget.yaml` | PodDisruptionBudget | Guarantees at least 1 backend pod stays up during *planned* disruptions (node drain, upgrade) — no protection against a node just crashing unplanned. |

## Kubernetes Installation - Local Docker Desktop's

### Installation commands, run in order

**Note:** this project uses only one cluster, `docker-desktop`, and that
cluster has only one Node, `desktop-control-plane` -- both are Docker
Desktop's own default names, assigned automatically by step 1, not
something this project configures. This project only defines the
namespace `docker-kubernetes-app` (`k8s/base/namespace.yaml`) on top of
that -- everything else (cluster, node, `ingress-nginx` and `kube-system`
namespaces) is shared, pre-existing infrastructure.

**Note:** `kubectl apply -k k8s/overlays/dev/` from step 6 is the single
command that actually creates every Kubernetes component this project
defines -- Ingress, Deployments, StatefulSet, DaemonSet, Job, CronJob,
Services, ConfigMap, Secret, ServiceAccount/RBAC, ResourceQuota/LimitRange,
HorizontalPodAutoscaler, PodDisruptionBudget, NetworkPolicy -- all of it, in one shot, because
`k8s/base/kustomization.yaml` lists every file explicitly (see "k8s files
purpose" below for what each one does). None of them are applied
individually or in a separate step.

This is the exact sequence that took this project from an already-empty
Docker Desktop Kubernetes cluster to the fully deployed, browser-reachable
app -- grouped by stage, in the order each stage actually has to run.

**1. Enable Kubernetes in Docker Desktop:**

Settings → Kubernetes → Enable Kubernetes → Apply & Restart. First-time
enable can take a few minutes; wait for a green "Kubernetes is running"
indicator, not just "starting."

**2. Confirm the cluster is actually reachable first (skip straight to step
3 if this already works):**

```sh
kubectl config get-contexts    # should list "docker-desktop"
kubectl config current-context # should print "docker-desktop" -- every command below targets this context
kubectl get nodes              # should show one node, STATUS: Ready
```

**3. Install the ingress-nginx controller (once per kubectl context, not
once per deploy -- skip if already installed):**

```sh
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.11.3/deploy/static/provider/cloud/deploy.yaml
kubectl -n ingress-nginx rollout status deployment/ingress-nginx-controller --timeout=180s
kubectl get svc -n ingress-nginx ingress-nginx-controller   # confirm EXTERNAL-IP is assigned, not <pending>
```

**4. Build the two real images (visible with `docker images`):**

```sh
docker build -t docker-kubernetes-app/backend:latest backend/
docker build -t docker-kubernetes-app/frontend:latest frontend/
```

**5. Tag them to match whichever overlay's `images:` block you're
deploying -- `dev` needs `:dev`, `prod` needs `:prod` (pick one, not
both):**

```sh
# prod:
docker tag docker-kubernetes-app/backend:latest docker-kubernetes-app/backend:prod
docker tag docker-kubernetes-app/frontend:latest docker-kubernetes-app/frontend:prod

# dev:
docker tag docker-kubernetes-app/backend:latest docker-kubernetes-app/backend:dev
docker tag docker-kubernetes-app/frontend:latest docker-kubernetes-app/frontend:dev
```

**6. Deploy the app itself, and wait for it to actually roll out (not just
for `apply` to not error) -- same pair of commands, pointed at whichever
overlay you tagged for in step 5:**

```sh
# dev:
kubectl kustomize k8s/overlays/dev/     # render + validate locally, no cluster needed
kubectl apply -k k8s/overlays/dev/      # actually apply this environment's version

# prod:
kubectl apply -k k8s/overlays/prod/

kubectl -n docker-kubernetes-app rollout status deployment/backend-deployment --timeout=180s
kubectl -n docker-kubernetes-app rollout status deployment/frontend-deployment --timeout=180s
kubectl -n docker-kubernetes-app get pods   # expect backend/frontend Running, database Running, db-migration Completed
```

**7. If Ingress is not working, force the `ingress-nginx-controller`
Service to republish from scratch.**

This is a separate failure point from `:4300` (see
"Routing without Ingress setup" vs "Routing with Ingress setup" above --
two fully independent `LoadBalancer` Services, so one working doesn't
guarantee the other does).

Deleting the Service forces Kubernetes to treat
it as a brand-new `LoadBalancer` request, which re-triggers
`cloud-provider-kind`'s entire "assign an IP and publish the port"
sequence fresh.

```sh
# reassign IP and ports for the Ingress controller
kubectl delete svc ingress-nginx-controller -n ingress-nginx
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.11.3/deploy/static/provider/cloud/deploy.yaml

# resolves Ingress's defined host/DNS name instead of localhost (at the OS level - see "Routing without Ingress setup"):
# File path:
#         C:\Windows\System32\drivers\etc\hosts on Windows
#         /etc/hosts on Linux/Mac
127.0.0.1 docker-kubernetes-app.local
```

Both routes will be still relevant without conflicts:

```sh
# localhost:4300 — without Ingress
Browser :4300 → WSL2/wslrelay :4300 → cloud-provider-kind :4300
  → Service "frontend-dev-localhost" (k8s/overlays/dev/frontend-localhost.yaml)
    port 4300 → targetPort 4200
  → frontend Pod directly, nginx:4200

No Ingress object is ever consulted. This Service selector points
straight at the frontend pods — it is a dev-only shortcut that exists
specifically to skip Ingress.

# localhost:80 (as docker-kubernetes-app.local) — with Ingress
Browser :80 → WSL2/wslrelay :80 → cloud-provider-kind :80
  → Service "ingress-nginx-controller" (installed separately, not part of this repo)
  → ingress-nginx pod reads the Ingress object rules (k8s/base/networking/ingress.yaml)
    "/api" → Service "backend" (port 8080)
    "/"    → Service "frontend" (port 80 → targetPort 4200)
  → same pods, same nginx:4200 / same backend:8080

This is the "real" production-shaped route — one hostname, path-based
routing, reaching both frontend and backend behind a single entry point.
```

That's the entire installation, start to finish -- six stages to deploy,
plus step 7 as a fallback if the Ingress route specifically needs it --
no steps skipped or reordered, matching the request-path diagram above.

## Docker-only: build and run backend + frontend

### Without Compose

No Kubernetes, no Compose -- just the two real images, built and run
directly with plain `docker build`/`docker run`, connected over a
hand-made network so frontend's nginx can still resolve `backend` by
name.  
This hello-world backend never actually queries Postgres in the first place --
so skipping the database here isn't cutting a corner, it's just skipping a
dependency this app doesn't really have:

```sh
docker build -t docker-kubernetes-app/backend:latest backend/
docker build -t docker-kubernetes-app/frontend:latest frontend/

docker network create app-net-manual

docker run -d --name backend --network app-net-manual docker-kubernetes-app/backend:latest
docker run -d --name frontend --network app-net-manual -p 8080:4200 \
  --user 101:101 --read-only --tmpfs /var/cache/nginx:mode=1777 --tmpfs /var/run:mode=1777 \
  docker-kubernetes-app/frontend:latest
```

The extra `frontend` flags aren't optional -- without them nginx (running as its
non-root image user) can't create `/var/cache/nginx/client_temp` and the
container exits immediately (`mkdir() ... Permission denied`). `mode=1777`
(world-writable, like `/tmp`) is required because `docker run --tmpfs` has no
option to set the mount's owner to match that user directly.

Open **http://localhost:8080**, click "Say hello to the backend", and
"Hello from Backend" comes back -- same round-trip as the Compose/Kubernetes
paths, just with nothing but plain Docker involved: no Compose file, no
cluster, no orchestration -- `frontend`'s nginx resolves `backend` purely
through the user-defined network's built-in DNS.

### With Compose

**The Compose equivalent exists too** (`docker/docker-compose.yml`), but
it is not "backend + frontend only": naming just those two services in the
command line below still pulls in `database` and `db-migration` as well,
since Compose always starts a named service's `depends_on` chain even when
that service isn't listed:

```sh
docker compose -f docker/docker-compose.yml up --build backend frontend   # starts database + db-migration too, via depends_on
```

If you actually want Compose to start only the two named services and
nothing else, the `depends_on` block under `backend` in
`docker/docker-compose.yml` would have to be removed first -- there's no
command-line flag that overrides it.

**Override Compose file:**  
The dev variant layers the override file on top and rebuilds on source changes:

```sh
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up --build --watch
```

**Profiling services/containers:**  
`db-backup` is opt-in: plain `up` never starts it, only `--profile tools` does.

```sh
docker compose -f docker/docker-compose.yml --profile tools run --rm db-backup > backup.sql
```

## Docker concepts

| Concept | Status | Where |
|---|---|---|
| **Image build (Dockerfile)** | | |
| Multi-stage builds | ✅ | backend, frontend, docker/worker Dockerfiles |
| `COPY --from=` (cross-stage copy) | ✅ | all multi-stage Dockerfiles |
| Build cache mounts (`--mount=type=cache`) | ✅ | backend (Maven `~/.m2`), frontend (npm), docker/worker (go build) |
| Build secrets (`--mount=type=secret`) | ✅ | docker/worker/Dockerfile |
| ARG vs ENV | ✅ | backend (`JAVA_VERSION`, `APP_VERSION`; ENV `APP_ENV`/`SERVER_PORT`/`JAVA_OPTS`), frontend (`NODE_VERSION`) |
| Heredoc `RUN <<EOF` | ✅ | backend/Dockerfile |
| `SHELL` instruction | ✅ | backend/Dockerfile |
| `COPY --chown=` | ✅ | backend/Dockerfile |
| `ADD` vs `COPY` (local archive extraction, remote URL fetch) | ✅ | docker/add-vs-copy/Dockerfile |
| `.dockerignore` | ✅ | backend, frontend, docker/worker, docker/onbuild-consumer |
| Multi-arch builds (`--platform`, `BUILDPLATFORM`/`TARGETOS`/`TARGETARCH`) | ✅ | frontend, docker/worker |
| `scratch` base / distroless mention | ✅ | docker/worker/Dockerfile |
| `ONBUILD` | ✅ | docker/onbuild-base + docker/onbuild-consumer |
| **Image metadata & runtime behavior (Dockerfile)** | | |
| OCI image LABELs | ✅ | backend, frontend |
| Non-root `USER` | ✅ | backend (creates the user explicitly, base image ships none), frontend |
| `VOLUME` | ✅ | backend/Dockerfile |
| `EXPOSE` | ✅ | backend, frontend |
| `STOPSIGNAL` | ✅ | backend, docker/worker |
| `HEALTHCHECK` | ✅ | backend, frontend, docker/worker |
| `ENTRYPOINT` + `CMD` (exec form) | ✅ | backend, docker/worker |
| **Compose: structure & configuration** | | |
| Docker Compose (services, `depends_on`, env_file, volumes, secrets) | ✅ | docker/docker-compose.yml |
| Compose `profiles:` (`--profile`) | ✅ | docker/docker-compose.yml (`db-backup`, gated behind `--profile tools`; not started by plain `up`) |
| Top-level `configs:` | ✅ | docker/docker-compose.yml (`items_schema` -> docker/migrations/create-items-table-script.sql, mounted into `db-migration`) |
| Multiple Compose files (`-f` override file) | ✅ | docker/docker-compose.dev.yml -- Compose counterpart of the k8s `dev` overlay |
| Compose Watch (`develop.watch`) and an active bind mount | ✅ | docker/docker-compose.dev.yml (`rebuild` watch on backend/frontend sources, read-only bind mount of `nginx.conf` in long syntax with `create_host_path: false`) |
| Compose `labels:` (`app.kubernetes.io/*`, shared via an `x-part-of` anchor) | ✅ | docker/docker-compose.yml (every service, network and volume) -- same keys as the k8s manifests |
| Compose `env_file` long form (`required: false`), `build.target`, `pull_policy` | ✅ | docker/docker-compose.yml (backend `.env.local` overrides; `target: runtime`; `pull_policy: missing` on the Postgres services) |
| Compose `include` / `extends` | ❌ | not needed: only one override file, nothing shared between projects |
| Other common Compose fields (`container_name`, `hostname`, `stop_signal`, `ulimits`, `deploy.replicas`, `pre_stop`/`post_start`, `build.platforms`/`cache_from`, `${VAR:?error}`, ...) | ❌ | listed as "Common ... missing" comments in docker/docker-compose.yml; no meaningful value in this setup |
| **Compose: startup order & shutdown** | | |
| `depends_on: condition: service_completed_successfully` (one-shot init/migration service) | ✅ | docker/docker-compose.yml (`db-migration` -> `backend`) -- Compose counterpart of the k8s `db-migration` Job |
| `init: true` (tini as PID 1), `stop_grace_period` | ✅ | docker/docker-compose.yml (backend) |
| `depends_on.restart: true`, `healthcheck.start_period` | ✅ | docker/docker-compose.yml (frontend -> backend, so nginx doesn't keep a stale backend IP; database, for initdb time) |
| **Compose: networking** | | |
| Custom Docker network | ✅ | docker/docker-compose.yml (`app-net`, bridge driver) |
| Network segmentation (several networks, `internal: true`) | ✅ | docker/docker-compose.yml (`app-net` for frontend+backend, internal `db-net` for backend+database+db-migration) |
| `expose:` and loopback-bound `ports:` (`127.0.0.1:8080:4200`) | ✅ | docker/docker-compose.yml (`expose` on backend and database; loopback binding on frontend) |
| `network_mode: "none"` (no network attached) | ✅ | docker/docker-compose.yml (worker -- reads only local `/proc`, no DB/HTTP calls) |
| **Compose: hardening & resource limits** | | |
| Compose runtime hardening (`read_only`, `tmpfs`, `cap_drop: [ALL]`, `security_opt: no-new-privileges`) | ✅ | docker/docker-compose.yml (`x-hardened` anchor on frontend, backend, db-migration, worker; database gets `no-new-privileges` only) -- Compose counterpart of the k8s `securityContext` |
| Compose resource limits (`deploy.resources.limits`, `cpus`/`memory`) | ✅ | docker/docker-compose.yml (frontend, backend, db-migration, database) -- same values as the k8s `limits:` blocks |
| Compose `deploy.resources.reservations` (memory), `pids_limit`, `user` | ✅ | docker/docker-compose.yml (memory reservations mirror the k8s `requests:`; `pids_limit` in `x-hardened`; `user: "70:70"` on db-migration) |
| **Distribution & supply chain** | | |
| Registry tagging/push, buildx cache export (`--cache-to`/`--cache-from`) | ❌ | CLI-only concepts, undemonstrated |
| Docker Content Trust / image signing | ❌ | not covered |

## Kubernetes concepts

✅ demonstrated directly · 🟡 exists in the running cluster, but only because another object here creates it automatically · ❌ not covered

| Concept | Status | Where |
|---|---|---|
| **Workload controllers** | | |
| Deployment (rolling update, replicas) | ✅ | controllers/backend-deployment.yaml, controllers/frontend-deployment.yaml |
| StatefulSet + `volumeClaimTemplates` | ✅ | controllers/database-statefulset.yaml |
| StatefulSet `updateStrategy` / `persistentVolumeClaimRetentionPolicy` | ✅ | controllers/database-statefulset.yaml |
| DaemonSet | ✅ | controllers/worker-daemonset.yaml |
| Job | ✅ | controllers/migration-job.yaml |
| CronJob | ✅ | controllers/cleanup-cronjob.yaml |
| Job `podFailurePolicy` / CronJob `timeZone`, `startingDeadlineSeconds` | ✅ | controllers/migration-job.yaml, controllers/cleanup-cronjob.yaml |
| **Pod spec: containers, health & volumes** | | |
| Probes: liveness/readiness/startup | ✅ | controllers/backend-deployment.yaml, controllers/frontend-deployment.yaml |
| initContainers | ✅ | controllers/backend-deployment.yaml |
| Sidecar container pattern (2 containers in one pod) | ❌ | only initContainer + single main container |
| Lifecycle hooks (`preStop`) | ✅ | controllers/backend-deployment.yaml, controllers/frontend-deployment.yaml, controllers/database-statefulset.yaml |
| emptyDir volumes | ✅ | controllers/backend-deployment.yaml, controllers/frontend-deployment.yaml |
| **Networking** | | |
| Service (ClusterIP) | ✅ | networking/backend-service.yaml, networking/frontend-service.yaml, networking/database-service.yaml (headless) |
| Service type LoadBalancer | ✅ | overlays/dev/frontend-localhost.yaml (dev only) |
| Service type NodePort | ❌ | only ClusterIP (base) and LoadBalancer (dev overlay) used |
| Ingress | ✅ | networking/ingress.yaml |
| NetworkPolicy (ingress+egress, default-deny) | ✅ | networking/networkpolicy.yaml |
| **Configuration & identity** | | |
| ConfigMap / `envFrom` | ✅ | config/configmap.yaml |
| Secret / `secretKeyRef` | ✅ | config/secret.yaml |
| ConfigMap/Secret mounted as volume (vs env) | ❌ | only used via env/envFrom |
| ServiceAccount + RBAC (Role/RoleBinding) | ✅ | config/role-based-access-control.yaml, config/serviceaccount.yaml |
| `imagePullSecrets` | ❌ | not needed (no private registry) but undemonstrated |
| **Security** | | |
| Pod Security Admission labels | ✅ | namespace.yaml (`enforce: baseline`, `warn`/`audit: restricted`) |
| SecurityContext (pod + container, `seccompProfile`, non-root, read-only root filesystem) | ✅ | every workload |
| **Resources, scaling & availability** | | |
| Resource requests/limits | ✅ | all workloads |
| ResourceQuota / LimitRange | ✅ | policy/resourcequota.yaml, policy/limitrange.yaml |
| HorizontalPodAutoscaler | ✅ | policy/horizontal-pod-autoscaler.yaml |
| PodDisruptionBudget | ✅ | policy/pod-disruption-budget.yaml |
| HorizontalPodAutoscaler scaling `behavior` / PodDisruptionBudget `unhealthyPodEvictionPolicy` | ✅ | policy/horizontal-pod-autoscaler.yaml, policy/pod-disruption-budget.yaml |
| **Scheduling & placement** | | |
| Affinity / anti-affinity | ✅ | controllers/backend-deployment.yaml |
| topologySpreadConstraints | ✅ | controllers/backend-deployment.yaml |
| Tolerations | ✅ | controllers/backend-deployment.yaml, controllers/worker-daemonset.yaml |
| **Organization & packaging** | | |
| Namespace | ✅ | namespace.yaml |
| Labels / annotations (`app.kubernetes.io/*`, `kubernetes.io/description`) | ✅ | every object |
| Kustomize overlays (`base` + `overlays`, patches) | ✅ | k8s/base/kustomization.yaml, k8s/overlays/dev/, k8s/overlays/prod/ |
| Helm chart | ❌ | not covered |
| **Objects Kubernetes creates or manages itself (not authored directly)** | | |
| Pod (bare) | 🟡 | created by the Deployments, StatefulSet, DaemonSet and Jobs |
| ReplicaSet (standalone) | 🟡 | created by each Deployment |
| ControllerRevision | 🟡 | created by the StatefulSet and DaemonSet to track revisions |
| PersistentVolumeClaim (standalone) | 🟡 | created by the StatefulSet's `volumeClaimTemplates` |
| Endpoints / EndpointSlice | 🟡 | created for each Service (EndpointSlice is the newer form) |
| Event | ❌ | written by Kubernetes itself |
| PodTemplate | ❌ | a standalone, reusable Pod template |
| ReplicationController | ❌ | the legacy predecessor of ReplicaSet |
| Binding | ❌ | assigns a Pod to a node, normally done by `kube-scheduler` |
| CSIStorageCapacity | ❌ | storage-driver capacity, published by CSI drivers |
| Lease | ❌ | leader election and node heartbeats (e.g. the `ingress-nginx` leader lease) |
| LocalSubjectAccessReview | ❌ | a one-off "may this user do X in this namespace?" check |
| ResourceClaim / ResourceClaimTemplate | ❌ | a request for special hardware such as GPUs, and a template that creates one per Pod |
