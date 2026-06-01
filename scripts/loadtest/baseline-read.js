import http from "k6/http";
import { check, sleep } from "k6";
import { Rate } from "k6/metrics";

const baseURL = __ENV.BASE_URL || "http://localhost:8080";
const expectRateLimit = (__ENV.EXPECT_RATE_LIMIT || "false").toLowerCase() === "true";
const unexpectedStatus = new Rate("unexpected_status_ratio");

export const options = {
  scenarios: {
    browse: {
      executor: "ramping-vus",
      startVUs: 2,
      stages: [
        { duration: "20s", target: 10 },
        { duration: "30s", target: 10 },
        { duration: "10s", target: 0 },
      ],
      gracefulRampDown: "5s",
    },
  },
  thresholds: {
    http_req_duration: ["p(95)<750"],
    unexpected_status_ratio: ["rate<0.02"],
    checks: [expectRateLimit ? "rate>0.90" : "rate>0.98"],
  },
};

function expectOK(path) {
  const res = http.get(`${baseURL}${path}`);
  const ok = res.status === 200 || (expectRateLimit && res.status === 429);
  unexpectedStatus.add(!ok);
  check(res, {
    [expectRateLimit ? `${path} status 200 or 429` : `${path} status 200`]: () => ok,
  });
}

export default function () {
  expectOK("/");
  expectOK("/measures");
  expectOK("/risks");
  sleep(0.4);
}
