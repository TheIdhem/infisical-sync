# infisical-sync

A CLI tool that reads a secrets inventory CSV, resolves variable values from
Ansible `group_vars` YAML files, and provisions them in Infisical with
corresponding AWS Secrets Manager or Parameter Store syncs.

---

## Concepts

### Environment hierarchy

Secrets are organised in two levels:

```
development  (base environment — holds the canonical values)
├── coffee
├── maison
├── fail
├── rocks
├── lol
├── wtf
├── rip
├── wine
├── red
└── ninja

sandbox      (base environment)
└── how

production   (base environment)
└── io
```

When you run the tool for a **category** (e.g. `development`):

1. All secrets are written to the base `development` environment.
2. For every individual env (coffee, maison, …):
   - The environment is created in Infisical if it does not exist.
   - A **secret import** is set up per folder: `coffee:/kafka-gap` ← `development:/kafka-gap`.
     This means `coffee` inherits all values from `development` automatically.
   - Empty folders are created so syncs have a valid source path.
   - An AWS app connection is created (or reused) for that env's AWS account.
   - AWS syncs are created per folder.

When you run for a **single env** (e.g. `rocks`):

- Secrets are written explicitly into the `rocks` environment.
- An AWS connection and syncs are created for `rocks` only.
- No secret import is set up (values are explicit, not inherited).

### Secret imports (inherit & override)

Individual envs import secrets from their base category environment using
Infisical's native **Secret Imports** feature. The resolution order is:

1. Explicit secret in the individual env (override) — highest priority.
2. Imported secret from the base category env — fallback.

To **override** a value for one env: add the secret explicitly in that env's
Infisical dashboard. The sync will push the override to AWS.

To **reset** an override back to the base value: delete the explicit secret
from the individual env in the Infisical dashboard. The import from the base
env takes over immediately on the next sync.

### AWS syncs

Each folder in Infisical maps to one AWS sync:

| CSV `Recommended Store` | Infisical sync type      | AWS destination                        |
|-------------------------|--------------------------|----------------------------------------|
| `Secrets Manager`       | aws-secrets-manager      | one JSON secret per folder (many-to-one) |
| `Parameter Store`       | aws-parameter-store      | path-prefix per folder                 |

Sync names follow the pattern `sm-{env}-{folder}` / `ps-{env}-{folder}`.
AWS destination names are prefixed with `--credential-prefix` (e.g. `theidhem/kafka-gap`).

### AWS connections

Each individual env has its own AWS app connection (`theidhem-aws-{env}`) using
IAM assume-role authentication. The role ARN is constructed automatically:

```
arn:aws:iam::{ACCOUNT_ID}:role/{AWS_ROLE_NAME}
```

Account IDs and regions are hardcoded per env in `config.go`.

### Safeguards

Running against `sandbox` (`how`), `production` (`io`), or their category
names requires typing the environment name to confirm before any API calls
are made. Dry-run skips the confirmation.

---

## Build

```bash
go build -o infisical-sync .
```

---

## Configuration

Credentials are read from environment variables (or `--flags`):

| Variable                    | Description                          |
|-----------------------------|--------------------------------------|
| `INFISICAL_CLIENT_ID`       | Universal Auth machine identity ID   |
| `INFISICAL_CLIENT_SECRET`   | Universal Auth machine identity secret |
| `AWS_ROLE_NAME`             | IAM role name (e.g. `InfisicalIntegrationRole`) — this role **must already exist** in the AWS account of each environment you are targeting |

Copy the sample and fill in your values:

```bash
cp .env.sample .env
# then edit .env
```

`.env` format (already gitignored):

```bash
INFISICAL_CLIENT_ID=...
INFISICAL_CLIENT_SECRET=...
AWS_ROLE_NAME=InfisicalIntegrationRole
```

Load it before running:

```bash
set -a && source .env && set +a
```

---

## Usage

```
./infisical-sync [flags]
```

| Flag                    | Default                                        | Description |
|-------------------------|------------------------------------------------|-------------|
| `--env`                 | *(required)*                                   | Env slug or category name (`coffee`, `development`, `io`, …) |
| `--csv`                 | `seeds/secrets-inventory.csv`                  | Path to secrets inventory CSV |
| `--group-vars-dir`      | `infra/ansible/group_vars`                     | Path to Ansible group_vars directory |
| `--credential-prefix`   | *(empty)*                                      | Prefix for all AWS secret/parameter names (e.g. `theidhem`) |
| `--project-name`        | `Engine-Mehdi`                                 | Infisical project name |
| `--aws-role-name`       | `$AWS_ROLE_NAME`                               | IAM role name for assume-role |
| `--aws-connection-name` | `theidhem-aws-{env}`                           | Override AWS connection name (single-env only) |
| `--aws-region`          | *(auto-detected)*                              | Override AWS region (single-env only) |
| `--base-url`            | `https://app.infisical.com`                    | Infisical base URL |
| `--dry-run`             | `false`                                        | Print actions without making any API calls |
| `--cleanup`             | `false`                                        | Destroy all secrets, syncs, imports, and connections |
| `--client-id`           | `$INFISICAL_CLIENT_ID`                         | Override machine identity client ID |
| `--client-secret`       | `$INFISICAL_CLIENT_SECRET`                     | Override machine identity secret |

---

## Examples

### Dry run — preview what would be created

```bash
cd /path/to/infisical-sync
set -a && source .env && set +a

./infisical-sync \
  --env development \
  --dry-run \
  --csv "seeds/secrets-inventory.csv" \
  --group-vars-dir infra/ansible/group_vars \
  --credential-prefix theidhem
```

### Provision full development category

Creates base secrets in `development` and sets up all individual envs
(coffee, maison, fail, rocks, lol, wtf, rip, wine, red, ninja) with
secret imports and AWS syncs.

```bash
./infisical-sync \
  --env development \
  --csv "seeds/secrets-inventory.csv" \
  --group-vars-dir infra/ansible/group_vars \
  --credential-prefix theidhem
```

### Provision a single environment

```bash
./infisical-sync \
  --env rocks \
  --csv "seeds/secrets-inventory.csv" \
  --group-vars-dir infra/ansible/group_vars \
  --credential-prefix theidhem
```

### Provision sandbox

```bash
./infisical-sync \
  --env sandbox \
  --csv "seeds/secrets-inventory.csv" \
  --group-vars-dir infra/ansible/group_vars \
  --credential-prefix theidhem
```

*(Requires typing `sandbox` to confirm)*

### Cleanup — destroy everything for development category

Deletes in order: all syncs (+ AWS resources) → secret imports → secrets →
folders → AWS connections. The base `development` environment's secrets and
folders are also removed.

```bash
./infisical-sync \
  --env development \
  --cleanup \
  --csv "seeds/secrets-inventory.csv" \
  --group-vars-dir infra/ansible/group_vars \
  --credential-prefix theidhem
```

### Cleanup — destroy a single environment

```bash
./infisical-sync \
  --env rocks \
  --cleanup \
  --csv "seeds/secrets-inventory.csv" \
  --group-vars-dir infra/ansible/group_vars \
  --credential-prefix theidhem
```
