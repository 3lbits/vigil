# Ad-hoc load tests (developer tool)

These scripts are **manual, occasional developer checks**. They are not part of
CI and are not intended as production-scale testing.

## Prerequisites

1. Install k6:
   ```sh
   brew install k6
   ```
2. Run Vigil locally with development stub auth enabled:
   ```sh
   DEV_STUB_AUTH=true make run
   ```

## Scenarios

- `baseline-read.js`  
  Read-path browse flow (`/`, `/measures`, `/risks`) with ramping VUs.
- `rate-limit-burst.js`  
  POST burst against `/dev/role` to confirm global limiter 429 behavior.

## Run

```sh
make loadtest-browse
make loadtest-rate-burst
```

Override base URL:

```sh
BASE_URL=http://localhost:8080 make loadtest-browse
```

## Two useful modes

1. Capacity baseline (disable global limiter on the app process):
   ```sh
   GLOBAL_RATE_LIMIT_PER_WINDOW=0 DEV_STUB_AUTH=true make run
   make loadtest-browse
   ```

2. Limiter verification (keep limiter enabled):
   ```sh
   DEV_STUB_AUTH=true make run
   EXPECT_RATE_LIMIT=true make loadtest-browse
   make loadtest-rate-burst
   ```

## Notes

- `rate-limit-burst.js` fetches CSRF once in `setup()` and reuses it for POSTs.
- The burst scenario is intentionally simple and repeatable for local debugging.
- Use these runs to compare configurations, not to derive hard production SLOs.
- k6 exits with code 99 when thresholds fail; this is expected when run conditions
  do not match the selected mode.
