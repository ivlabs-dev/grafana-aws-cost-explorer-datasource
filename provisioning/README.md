# Local provisioning

The standard development container mounts this directory at
`/etc/grafana/provisioning`.

- `datasources/datasources.yml` creates an editable AWS Cost Explorer data
  source with the default AWS credential chain and no committed credentials.
- `dashboards/dashboards.yml` loads the JSON files in `dashboards/json`.
- `dashboards/json/aws-cost-explorer.json` demonstrates total daily cost, cost
  by service, cost by linked account, and a monthly amortized-cost table.

Export AWS variables before `docker compose up` if you want the container to
use environment credentials. For production, prefer the Grafana workload's
IAM role, ECS/EC2 role, or Kubernetes IRSA. Never add real credentials to these
files.
