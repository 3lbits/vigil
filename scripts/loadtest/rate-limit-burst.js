import http from "k6/http";
import { check } from "k6";
import { Counter, Rate } from "k6/metrics";

const baseURL = __ENV.BASE_URL || "http://localhost:8080";
const burstPath = __ENV.BURST_PATH || "/dev/role";
const role = __ENV.DEV_ROLE || "admin";

const status429 = new Counter("status_429_total");
const status403 = new Counter("status_403_total");
const rateLimited = new Rate("rate_limited_ratio");
const unexpectedStatus = new Rate("unexpected_status_ratio");

export const options = {
  scenarios: {
    burst: {
      executor: "ramping-vus",
      stages: [
        { duration: "10s", target: 40 },
        { duration: "20s", target: 40 },
        { duration: "10s", target: 0 },
      ],
      gracefulRampDown: "3s",
    },
  },
  thresholds: {
    rate_limited_ratio: ["rate>0.01"],
    unexpected_status_ratio: ["rate<0.02"],
    checks: ["rate>0.98"],
  },
};

export function setup() {
  const warmup = http.get(`${baseURL}/`);
  if (warmup.status !== 200) {
    throw new Error(`setup GET / failed with status ${warmup.status}. Run with DEV_STUB_AUTH=true.`);
  }

  const csrfCookie = warmup.cookies._csrf;
  if (!csrfCookie || csrfCookie.length === 0) {
    throw new Error("setup could not read _csrf cookie from GET /");
  }
  return { csrf: csrfCookie[0].value };
}

export default function (data) {
  const body = `_csrf=${encodeURIComponent(data.csrf)}&role=${encodeURIComponent(role)}`;
  const res = http.post(`${baseURL}${burstPath}`, body, {
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    cookies: { _csrf: data.csrf },
    redirects: 0,
  });

  const limited = res.status === 429;
  const blocked = res.status === 403;
  const ok = res.status === 303 || limited || blocked;
  rateLimited.add(limited);
  unexpectedStatus.add(!ok);
  if (limited) {
    status429.add(1);
  }
  if (blocked) {
    status403.add(1);
  }

  check(res, {
    "status is 303, 429, or 403": () => ok,
  });
}
