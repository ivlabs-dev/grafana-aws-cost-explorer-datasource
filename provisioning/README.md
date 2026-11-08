# Local provisioning

The standard development container mounts this directory at
`/etc/grafana/provisioning`.

- `datasources/datasources.yml` creates an editable AWS Cost Explorer data
  source. Its authentication mode comes from `AWS_COST_EXPLORER_AUTH_MODE`,
  which `docker-compose.yaml` defaults to `mock` (built-in synthetic data, no
  AWS account needed). With `static`, its secure fields are populated from AWS
  environment variables.
- `alerting/commitment-expiry.yml` provisions two example alert rules, in the
  `AWS Cost Explorer` folder, that fire when a Reservation or Savings Plan
  expires within 30 days. With Mock data they fire for one Reservation and one
  Savings Plan, so they work as a demo out of the box.
- `dashboards/dashboards.yml` loads the JSON files in `dashboards/json`.
- `dashboards/json/aws-cost-explorer.json` demonstrates cost-in-period,
  month-to-date, and change-vs-previous-period stats, total daily cost, cost
  by service (with tax split onto its own axis), the top 10 cost services,
  cost by linked account, a monthly amortized-cost table, combined
  Reservation/Savings Plans utilization-vs-coverage trend charts, and
  Reservation/Savings Plans inventory tables. It mirrors the dashboard
  bundled with the plugin itself at `src/dashboards/overview.json`.

To query a real account, export the AWS variables and
`AWS_COST_EXPLORER_AUTH_MODE=static` before `docker compose up`. Grafana copies
the credentials into the data source's encrypted `secureJsonData`; the plugin
does not resolve ambient credentials through the AWS SDK. Never add real
credentials to these files.
