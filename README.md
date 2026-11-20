# FinFocus CloudZero Plugin

FinFocus plugin that reads historical cost, budgets, and optimization
recommendations from the CloudZero REST API v2. Cost covers AWS, Azure, and GCP resources when a resource-level
CloudZero dimension is configured.

## v0.1.0 limitations

This release is built from CloudZero's public docs and tested with fake HTTP
responses. On 2026-11-17 it was checked against a live CloudZero account with
one AWS stack (EC2 instances, NAT gateways, and Elastic IPs in `us-west-2`).
That check verified the bare-key auth, the costs response shape, one row per
resource in the `Resource` dimension, the CZRN format for those AWS types,
and the `AWS` value of `CloudProvider`. Everything else below is still
unverified.

- Without `CLOUDZERO_RESOURCE_DIMENSION` the plugin declines the resource. It
  does not return an account or tag slice, because that slice would be counted
  once per resource in a stack total. `Resource` is the dimension that worked
  on the live account.
- The plugin does not filter on `CloudProvider`. It groups by it and matches
  each row's value to the resource's provider ignoring case. That worked for
  `AWS`. Azure and GCP spellings are unverified, but none is hardcoded.
- CloudZero bills an instance's root EBS volume as its own `volume` CZRN. When
  the instance's attributes name it (`rootBlockDevice.volumeId`, as Pulumi
  state does), the volume's cost is added to the instance as a separate
  `cloudzero:<cost type> <currency> root volume` line, from the same query.
  Data volumes are not added.
- The public IPv4 charge of an Elastic IP lands on its network interface, not
  on the EIP, and is not reported for the EIP.
- The costs response has no currency. Results are labelled with
  `CLOUDZERO_CURRENCY`, which defaults to `USD`.
- A bad key is documented as 403. The client treats 401 the same way.
- There is no `Retry-After` or `X-RateLimit-*` header in the docs.

Live checks belong to the owner. `make test-integration` skips unless
`CLOUDZERO_API_KEY` is set, and it makes at most three calls.

## What it answers

| RPC | Behavior |
| --- | --- |
| GetActualCost | Daily rows from `GET /v2/billing/costs` |
| BatchCost | Actual cost only. One upstream call per filter group |
| GetBudgets | CloudZero's monthly budgets with this month's spend, from `GET /v2/budgets` |
| GetRecommendations | CloudZero's recommendations for the requested resources, from `GET /v2/optimize/recommendations` |
| GetProjectedCost, GetPricingSpec, EstimateCost | `Unimplemented` |
| DismissRecommendation, GetRecommendationHistory | `Unimplemented`. Core keeps dismissals itself |
| Supports | Actual cost for AWS, Azure, and GCP when a key and a resource dimension are set. Budgets and recommendations when a key is set |

`GetPluginInfo` advertises `ACTUAL_COSTS`, `BATCH_COST`, `BUDGETS`, and
`RECOMMENDATIONS`. The plugin
reports the name `cloudzero`, its registry name, so `--adapter cloudzero`
selects it.

An empty result means CloudZero returned no rows. The plugin does not invent
a `$0` cost. Each row's `source` names CloudZero, the cost type, and the
configured currency. `expires_at` is computed here. Closed months use
`CLOUDZERO_CACHE_TTL` (default 6 hours). The newest two days use
`CLOUDZERO_RECENT_TTL` (default 1 hour).

### Budgets

`GetBudgets` lists the budgets kept in CloudZero. Each one's limit is its
planned amount for the current UTC month, in `CLOUDZERO_CURRENCY`, and its
status is the month's actual spend that CloudZero reports. FinFocus core works
out the percentage used, the forecast, and the health from those two numbers.
CloudZero alert percentages that are switched on become actual-spend
thresholds. The CloudZero view a budget covers is kept in its metadata as
`cloudzero_view_id`.

Budgets that are not monthly, or that plan nothing for the current month, are
left out, and a debug log line counts them. A budget with no granularity counts
as monthly. The list comes from one call, cached for `CLOUDZERO_RECENT_TTL`, and
that call spends one request from `CLOUDZERO_DAILY_BUDGET`. The budgets
response has not been seen with data from a live account yet.

A budget's source is `cloudzero`, so a FinFocus provider filter such as `aws`
hides it: a CloudZero budget can cover several providers.

### Recommendations

`GetRecommendations` returns the optimization recommendations CloudZero has
made for the resources core asks about. Each recommendation names its resource
as a CZRN, which is matched to core's target resources the same way cost rows
are: from `pulumi:cloudId`, an ARN (the descriptor's, or its `pulumi:arn` or
`arn` tag), or a bare AWS id such as `vol-0abc`. A match carries the target's
id, so core attaches it to that resource. With no targets, every
recommendation is returned under its CZRN. No resource dimension is needed.

The plugin asks for every recommendation worth at least $0.01 over the last
30 days. CloudZero's own default floor is $1.00, which hid every
recommendation on the account this was checked against. Raise the floor in
core with its minimum-savings filter. The estimated savings are CloudZero's
30-day cost impact, taken as monthly and scaled for `daily` or `annual`
requests, in `CLOUDZERO_CURRENCY`. CloudZero's title is the description, and
its recommendation type, effort, status, accounts, and resource CZRN
(`cloudzero_resource`) are kept in metadata.
The plugin does not guess an action type or a priority.

CloudZero's `ignored` recommendations count as dismissed: core sees them only
when it asks for dismissed recommendations. `addressed` ones are never
returned. Dismissing a recommendation in FinFocus is kept by core; the plugin
never changes anything in CloudZero.

The list comes from one call, cached for `CLOUDZERO_RECENT_TTL` and shared by
every page and batch core asks for. That call spends one request from
`CLOUDZERO_DAILY_BUDGET`. Only CloudZero's "Upgradable EBS Volumes" type has
been seen live.

A recommendation on an EC2 instance's root volume is shown on the instance,
as actual cost adds the root volume's cost to it. CloudZero names the volume,
which is not a resource in the stack, so the plugin reads the instance's
`rootBlockDevice.volumeId` from the attributes core sends with the request;
`cloudzero_resource` still names the disk. This needs a FinFocus core that
sends attributes on recommendation targets (rshade/finfocus#2281). With an
older core, root-volume recommendations are not shown. Data volumes are not
attached.

## Configuration

The process starts without a key. Cost calls then return `FailedPrecondition`.

| Variable | Default | Purpose |
| --- | --- | --- |
| `CLOUDZERO_API_KEY` | empty | API key. Required for cost calls. Never logged |
| `CLOUDZERO_BASE_URL` | `https://api.cloudzero.com/v2` | API origin, including `/v2` |
| `CLOUDZERO_COST_TYPE` | `real_cost` | One of the eight documented cost types |
| `CLOUDZERO_CURRENCY` | `USD` | Label only. The API does not return a currency |
| `CLOUDZERO_RESOURCE_DIMENSION` | empty | Dimension ID that identifies one resource |
| `CLOUDZERO_TAG_DIMENSIONS` | empty | `tagKey=dimensionID` pairs, comma-separated |
| `CLOUDZERO_DAILY_BUDGET` | `50` | Requests this process will make, retries included. Cursor pages are free |
| `CLOUDZERO_CACHE_TTL` | `6h` | Cache lifetime for older days |
| `CLOUDZERO_RECENT_TTL` | `1h` | Cache lifetime for the newest two days |

`real_cost` is CloudZero's default. It keeps consumption charges and drops
tax, support, and unused commitments, so totals will not match an invoice or
the AWS Cost Explorer plugin.

The key needs the fixed scopes `billing:read_costs` and
`billing:read_dimensions`. Add `billing:read_resource_tags` only if you also
read tags. The budgets and recommendations pages name no scope; a key with
these scopes read both on 2026-11-19. The costs call sends the key as a bare `Authorization` value.
There is no `Bearer` prefix.

`CLOUDZERO_TAG_DIMENSIONS` does not invent dimension IDs from tag names. Map
a resource tag to a dimension ID you already created in CostFormation, for
example `Team=User:Defined:Team`.

The Resource dimension is matched with a CloudZero resource name (CZRN).
CloudZero does not document the CZRN format. Values on the live account look
like `czrn:aws:ec2:us-west-2:<account>:instance:i-abc`.
An id that already starts with `czrn:` is used unchanged. An AWS ARN is
converted: the service and resource type come from the ARN, `natgateway`
becomes `nat-gateway`, and an empty region becomes `cross-region`.

Some AWS resources have no ARN in Pulumi state, such as a NAT gateway. A bare
id with a known prefix (`i-`, `vol-`, `eni-`, `nat-`, `eipalloc-`) is matched
on the end of the CZRN, `:<type>:<id>`, within the resource's region. Its
query filters on provider and region only, because the account is unknown.
An Azure or GCP id is not converted. Supply a `czrn:` value for those.

## CloudZero call shape

```text
GET https://api.cloudzero.com/v2/billing/costs
  ?start_date=2023-10-01T00:00:00Z
  &end_date=2023-11-01T00:00:00Z
  &granularity=daily
  &group_by=Resource
  &group_by=CloudProvider
  &filters={"Account":["123456789012"],"Region":["us-east-1"]}
  &cost_type=real_cost
```

The documented costs limit is 60 requests a day. Cursor pagination does not
count. A 429 becomes `ResourceExhausted` with no retry loop; its message
names the wait CloudZero states, such as "retry in 24 minutes". For 5 minutes
after a 429, the plugin fails every new query with the same error without
sending it and without spending budget; a query already paging finishes. A 410 restarts the query once.
Transport errors and 5xx are retried briefly, and each retry spends one
request from `CLOUDZERO_DAILY_BUDGET`. The client timeout is 30 seconds.

Queries are aligned to the UTC month that covers the request. The resource is
not a filter: the query is grouped by the resource dimension, and the plugin
keeps the rows for the requested resource and days. One call, and one cache
entry, serves every resource with the same provider, account, region, and tag
dimensions. This holds for concurrent requests too: when FinFocus asks for
several such resources at once, the first request sends the call and the
others wait for its result. A request that gives up waiting does not stop the
call. A failed call returns its error to every waiting request and is not
cached.

## Build

Requires Go 1.27.1 or later.

```bash
make build
make test
make test-race
make lint
make test-integration   # skips without CLOUDZERO_API_KEY
```

## Routing

If aws-ce and this plugin are both installed, both can answer actual cost for
AWS. Which result core shows, when no routing rule is set, is not verified
here. Set a routing rule if both plugins are installed.

## License

Apache License 2.0. See [LICENSE](LICENSE).
