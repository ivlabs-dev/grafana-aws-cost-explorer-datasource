# Local provisioning

The standard development container mounts this directory at
`/etc/grafana/provisioning`.

- `datasources/datasources.yml` creates an editable AWS Cost Explorer data
  source whose secure fields are populated from AWS environment variables.
- `dashboards/dashboards.yml` loads the JSON files in `dashboards/json`.
- `dashboards/json/aws-cost-explorer.json` demonstrates total daily cost, cost
  by service, cost by linked account, and a monthly amortized-cost table.

Export AWS variables before `docker compose up` if you want the container to
provision working credentials. Grafana copies them into the data source's
encrypted `secureJsonData`; the plugin does not resolve ambient credentials
through the AWS SDK. Never add real credentials to these files.
