# Local provisioning

The standard development container mounts this directory at
`/etc/grafana/provisioning`.

- `datasources/datasources.yml` creates an editable AWS Cost Explorer data
  source whose secure fields are populated from AWS environment variables.
- `dashboards/dashboards.yml` loads the JSON files in `dashboards/json`.
- `dashboards/json/aws-cost-explorer.json` demonstrates cost-in-period,
  month-to-date, and change-vs-previous-period stats, total daily cost, cost
  by service (with tax split onto its own axis), the top 10 cost services,
  cost by linked account, a monthly amortized-cost table, combined
  Reservation/Savings Plans utilization-vs-coverage trend charts, and
  Reservation/Savings Plans inventory tables. It mirrors the dashboard
  bundled with the plugin itself at `src/dashboards/overview.json`.

Export AWS variables before `docker compose up` if you want the container to
provision working credentials. Grafana copies them into the data source's
encrypted `secureJsonData`; the plugin does not resolve ambient credentials
through the AWS SDK. Never add real credentials to these files.
