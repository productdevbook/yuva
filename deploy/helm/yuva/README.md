# Yuva Helm chart

Runs [Yuva](https://github.com/productdevbook/yuva) on Kubernetes: the `ghcr.io/productdevbook/yuva`
image, database migrations in an init container, a Service, and optionally an Ingress, volumes and
a Prometheus ServiceMonitor. The chart's version is Yuva's version.

The chart brings no database. Give it a Postgres 16+ URL: a managed database, or a cluster run by
an operator such as [CloudNativePG](https://cloudnative-pg.io).

## Install

1. Make a Secret with the required settings and any other secrets:

   ```sh
   kubectl create namespace yuva
   kubectl -n yuva create secret generic yuva \
     --from-literal=YUVA_DATABASE_URL='postgres://yuva:<password>@<host>:5432/yuva?sslmode=require' \
     --from-literal=YUVA_MASTER_KEY="$(openssl rand -base64 32)" \
     --from-literal=YUVA_INGRESS_SECRET="$(openssl rand -hex 32)" \
     --from-literal=YUVA_SMTP_USERNAME='yuva@example.com' \
     --from-literal=YUVA_SMTP_PASSWORD='<password>'
   ```

   Keep a copy of `YUVA_MASTER_KEY` outside the cluster: without it a restored database loses
   the secrets Yuva stores (SMTP passwords of channels, identity and webhook secrets).

2. Write `values.yaml`:

   ```yaml
   publicUrl: https://support.example.com
   secrets:
     existingSecret: yuva
   env:
     YUVA_CLIENT_IP_HEADER: X-Real-IP
     YUVA_SMTP_HOST: smtp.example.com
     YUVA_SMTP_FROM: Yuva <yuva@example.com>
   ingress:
     enabled: true
     className: nginx
     annotations:
       nginx.ingress.kubernetes.io/proxy-body-size: 30m
       nginx.ingress.kubernetes.io/proxy-read-timeout: "120"
       nginx.ingress.kubernetes.io/proxy-send-timeout: "120"
     tls:
       - secretName: yuva-tls
         hosts: [support.example.com]
   ```

3. Install:

   ```sh
   helm install yuva oci://ghcr.io/productdevbook/charts/yuva --version 0.2.0 -n yuva -f values.yaml
   ```

4. Create the first owner:

   ```sh
   kubectl -n yuva exec deploy/yuva -c yuva -- \
     /yuva bootstrap --email you@example.com --workspace "Example" --name "Your Name"
   ```

The other [operator commands](https://github.com/productdevbook/yuva/blob/main/docs/operations.md#operator-commands) run the same way.

## Settings

Every `YUVA_*` setting in [Configuration](https://github.com/productdevbook/yuva/blob/main/docs/configuration.md) goes in `env`, except
these, which the chart sets:

| Variable | From |
|---|---|
| `YUVA_PUBLIC_URL` | `publicUrl` (required; rendering fails without it) |
| `YUVA_STORAGE`, `YUVA_STORAGE_DIR` | `storage.driver` |
| `YUVA_METRICS_ADDR` | `metrics.enabled` (`:9090` or `off`) |
| `YUVA_GEOIP_DIR` | `/data/geoip`, unless set in `env` |
| `GOMEMLIMIT` | `goMemLimitRatio` (0.9) of `resources.limits.memory`, unless set in `env` |

Secrets are read from `secrets.existingSecret` with `secretKeyRef`s, one key per variable:
`YUVA_DATABASE_URL` and `YUVA_MASTER_KEY` (required), `YUVA_INGRESS_SECRET`, `YUVA_SMTP_USERNAME`,
`YUVA_SMTP_PASSWORD`, `YUVA_S3_ACCESS_KEY_ID`, `YUVA_S3_SECRET_ACCESS_KEY`, `YUVA_VAPID_PUBLIC_KEY`,
`YUVA_VAPID_PRIVATE_KEY`, `YUVA_MICROSOFT_CLIENT_ID`, `YUVA_MICROSOFT_CLIENT_SECRET`, `YUVA_SES_ACCESS_KEY_ID` and
`YUVA_SES_SECRET_ACCESS_KEY` (optional).
`secrets.keys` renames a key, for example `YUVA_DATABASE_URL: uri` for the app Secret CloudNativePG
makes. Putting one of them in `env` fails the rendering.

For a quick try, leave `existingSecret` empty and put the values in `secrets.values`; the chart
then makes the Secret. This is less safe: the values live in your values file and in the Helm
release, readable by anyone who can read Secrets in the namespace. Other keys in `secrets.values`
become variables too.

Web Push needs a VAPID key pair: run
`docker run --rm ghcr.io/productdevbook/yuva:0.2.0 vapid-keys` and add both values to the Secret.

## Storage

- `storage.driver: local` (default) keeps attachments on a PersistentVolumeClaim
  (`persistence.attachments`, 10 GiB, `ReadWriteOnce`) at `/data/attachments`. It works with one
  replica only, and the Deployment uses the `Recreate` strategy so the volume is never mounted
  twice.
- `storage.driver: s3` for S3, R2 or MinIO: set `YUVA_S3_ENDPOINT`, `YUVA_S3_BUCKET` (and
  `YUVA_S3_REGION`, `YUVA_S3_PATH_STYLE`) in `env` and the keys in the Secret. Needed for more than
  one replica.
- `persistence.geoip.enabled` keeps the monthly DB-IP Lite download for analytics on a volume;
  otherwise each new pod downloads it again into an `emptyDir`.

The chart's PVCs carry `helm.sh/resource-policy: keep`: `helm uninstall` leaves them, and your
attachments, in place.

The root filesystem is read-only; uploads are buffered in an `emptyDir` at `/tmp`
(`tmp.sizeLimit`, 2 GiB). Raise it if teams upload large export files when
[moving a mailbox](https://github.com/productdevbook/yuva/blob/main/docs/migrate-files.md).

## Several replicas

`replicaCount` above 1 is supported with `storage.driver: s3`: jobs, realtime events and presence
go through Postgres, and so do rate limits: N replicas allow what one does. The chart then adds a PodDisruptionBudget (`podDisruptionBudget.minAvailable`,
1) and rolls updates one pod at a time; `topologySpread.enabled` spreads pods over nodes, and with
`zoneSpread` over zones. Migrations run in each pod's init container under a Postgres lock, so
pods starting together are safe.

## Ingress and sticky sessions

Yuva needs no sticky sessions. The Ingress routes the host of `publicUrl` (or `ingress.host`, plus
`ingress.extraHosts`, e.g. the host of `YUVA_MAIL_LINK_URL`) to the Service. The proxy must pass
WebSocket upgrades on `/v1/realtime` and `/client/v1/realtime`, keep idle connections longer than
60 seconds and accept bodies of at least 26 MiB.

If your load balancer sets a sticky-session cookie anyway, it must not set it on the analytics
script and its endpoint, which promise to leave nothing on the visitor's device. Set
`service.analytics.separateService: true`: `/yuva-analytics.js`, `/yuva-analytics-consent.js` and
`/client/v1/analytics/` then go through a second Service (`service.analytics.annotations`) and a second Ingress
(`ingress.analyticsAnnotations`), so put your stickiness annotations only in
`service.annotations` and `ingress.annotations`.

## Metrics

Prometheus metrics are served on port 9090 at `/metrics`, never through the Ingress.
`metrics.serviceMonitor.enabled` adds a metrics Service and a ServiceMonitor for the Prometheus
Operator.

## Values

| Key | Default | Meaning |
|---|---|---|
| `publicUrl` | `""` | Required. The URL people and apps use. |
| `image.repository`, `image.tag`, `image.digest`, `image.pullPolicy` | `ghcr.io/productdevbook/yuva`, appVersion, `""`, `IfNotPresent` | Image; a digest wins over the tag. |
| `replicaCount` | `1` | Pods. More than 1 needs `storage.driver: s3`. |
| `updateStrategy` | `{}` | Deployment strategy; `Recreate` with local storage, else rolling one at a time. |
| `env` | `{}` | Plain `YUVA_*` settings. |
| `extraEnv` | `[]` | Extra env entries, e.g. `valueFrom`. |
| `secrets.existingSecret`, `secrets.keys`, `secrets.values` | `""`, `{}`, `{}` | See Settings. |
| `migrations.enabled`, `migrations.resources` | `true`, `{}` | The `yuva migrate up` init container. Disabled, run migrations yourself before upgrading. |
| `storage.driver` | `local` | `local` or `s3`. |
| `persistence.attachments.*` | 10Gi RWO | `existingClaim`, `storageClass` (`-` for none), `accessModes`, `size`, `annotations`. |
| `persistence.geoip.*` | disabled, 1Gi RWO | `enabled` and the same keys. |
| `tmp.sizeLimit`, `tmp.medium` | `2Gi`, `""` | The `/tmp` emptyDir. |
| `service.type`, `service.port`, `service.annotations` | `ClusterIP`, `80`, `{}` | Main Service. |
| `service.analytics.separateService`, `service.analytics.annotations` | `false`, `{}` | Analytics Service and Ingress without stickiness. |
| `ingress.enabled`, `ingress.className`, `ingress.annotations`, `ingress.host`, `ingress.extraHosts`, `ingress.tls`, `ingress.analyticsAnnotations` | off | Ingress. |
| `metrics.enabled` | `true` | Port 9090. |
| `metrics.serviceMonitor.enabled`, `.interval`, `.scrapeTimeout`, `.labels` | off, `30s` | ServiceMonitor. |
| `goMemLimitRatio` | `0.9` | Share of the memory limit for `GOMEMLIMIT`. |
| `resources` | 100m / 256Mi requests, 1Gi memory limit | Container resources. |
| `livenessProbe`, `readinessProbe`, `startupProbe` | `/healthz`, `/readyz`, `/healthz` | Probes. |
| `terminationGracePeriodSeconds` | `30` | |
| `podDisruptionBudget.minAvailable`, `.maxUnavailable` | `1`, `""` | Used when `replicaCount` > 1. |
| `topologySpread.enabled`, `.maxSkew`, `.whenUnsatisfiable`, `.zoneSpread` | off, `1`, `ScheduleAnyway`, off | Spread over nodes and zones. |
| `serviceAccount.create`, `.name`, `.annotations` | `true` | The pod's ServiceAccount; its token is not mounted. |
| `podAnnotations`, `podLabels`, `podSecurityContext`, `securityContext` | non-root 65532, read-only root, no capabilities | Pod settings. |
| `nodeSelector`, `tolerations`, `affinity`, `priorityClassName`, `imagePullSecrets` | empty | Scheduling. |
