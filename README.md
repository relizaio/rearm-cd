# ReARM CD

ReARM CD is a tool that acts as an agent on the Kubernetes side to connect the instance to [ReARM](https://rearmhq.com). The deployments to the instance may be then controlled from ReARM.

The recommended way to install is to use included Helm Chart (Will be available soon).

## Prerequisites

ReARM CD requires [Bitnami Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) to be installed in your cluster before installation.

Install Sealed Secrets using Helm:
```bash
helm install sealed-secrets -n kube-system --set-string fullnameOverride=sealed-secrets-controller oci://registry.relizahub.com/library/sealed-secrets
```

For more information, see the [Sealed Secrets Helm Chart documentation](https://github.com/bitnami-labs/sealed-secrets#helm-chart).

## Installation via Helm Chart
1. Create namespace
```
kubectl create namespace rearm-cd
```
2. Create secret
```
kubectl create secret generic rearm-cd --from-literal=REARM_APIKEYID=your-rearm-api-id --from-literal=REARM_APIKEY=your-rearm-api-key --from-literal=REARM_URI=your-rearm-uri -n rearm-cd
```
3. Install Helm Chart
```
helm install -n rearm-cd rearm-cd oci://registry.relizahub.com/library/rearm-cd
```

## RBAC Configuration

By default, ReARM CD is installed with cluster-wide permissions (ClusterRole and ClusterRoleBinding). You can customize the RBAC configuration via Helm values:

### Disable RBAC Resource Creation

If you want to manage RBAC resources separately:

```yaml
rbac:
  createServiceAccount: false
  createClusterRole: false
  createClusterRoleBinding: false
  serviceAccountName: "my-existing-service-account"
```

### Namespace-Scoped Permissions

To restrict ReARM CD to specific namespaces instead of cluster-wide access, set `rbac.namespaces`:

```yaml
rbac:
  namespaces:
    - rearm-cd      # Required: ReARM CD's own namespace
    - default
    - staging
    - production
```

When `rbac.namespaces` is set:
- Namespace-scoped **Roles** and **RoleBindings** are created in each listed namespace
- **ClusterRole** and **ClusterRoleBinding** are not created
- ReARM CD cannot access resources outside the listed namespaces
- **Important:** You must include the namespace where ReARM CD itself runs (the release namespace) for normal operations

### Security Considerations

- Namespace-scoped RBAC prevents privilege escalation — ReARM CD cannot create ClusterRoles or grant itself access to other namespaces
- Kubernetes RBAC is deny-by-default and additive only
- ReARM CD requires full permissions (`*` verbs on `*` resources) within its allowed namespaces to manage Helm charts, secrets, and deployments

### Sample Full RBAC controlled Installation for ReARM deployment
Create rearm-cd-values.yaml with the following content:

```yaml
rbac:
  createClusterRole: false
  createClusterRoleBinding: false
  namespaces:
    - rearm-cd
    - rearm
    - dtrack
```

```bash
kubectl create ns rearm-cd rearm dtrack
helm install sealed-secrets -n kube-system --set-string fullnameOverride=sealed-secrets-controller oci://registry.relizahub.com/library/sealed-secrets
kubectl create secret generic rearm-cd --from-literal=REARM_APIKEYID=your-rearm-api-id --from-literal=REARM_APIKEY=your-rearm-api-key --from-literal=REARM_URI=your-rearm-uri -n rearm-cd
helm install -n rearm-cd -f rearm-cd-values.yaml rearm-cd oci://registry.relizahub.com/library/rearm-cd
```

## Dry Run Mode

To enable dry run mode, set the `DRY_RUN` environment variable to `true`:

```
DRY_RUN=true
```

In this mode, ReARM CD will log all mutating helm and kubectl commands (install, upgrade, uninstall, delete, create namespace) but will not execute them. Read-only operations such as chart downloads, value merging, and metadata streaming will continue to run normally.

## Debug Logging

To enable debug level logging, set the `LOG_LEVEL` environment variable to `debug`:

```
LOG_LEVEL=debug
```

This will output additional diagnostic information such as custom values resolution details and other internal state.

## Workspace Backup to S3

ReARM CD can periodically back up the workspace directory to an S3 bucket. Backups are encrypted with AES-256-CBC before upload.

To enable, set the following environment variables:

| Variable | Required | Description |
|---|---|---|
| `BACKUP_ENABLED` | Yes | Set to `true` to enable backups |
| `BACKUP_SCHEDULE` | Yes | Cron schedule expression (e.g. `0 2 * * *` for daily at 2 AM) |
| `AWS_REGION` | Yes | AWS region of the S3 bucket |
| `AWS_BUCKET` | Yes | S3 bucket name |
| `ENCRYPTION_PASSWORD` | Yes | Password used for AES-256-CBC encryption |
| `AWS_ACCESS_KEY_ID` | No | AWS access key (falls back to default AWS credential chain) |
| `AWS_SECRET_ACCESS_KEY` | No | AWS secret key (falls back to default AWS credential chain) |
| `BACKUP_PREFIX` | No | Prefix for backup file names in S3 |

The backup process:
1. Creates a tar.gz archive of the workspace directory
2. Encrypts it using `openssl enc -aes-256-cbc -a -pbkdf2 -iter 600000 -salt`
3. Uploads the encrypted file to the specified S3 bucket