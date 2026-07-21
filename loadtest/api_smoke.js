// api_smoke.js — smoke/load test for POST /v1/jobs against a taskflow deployment
// started via docker-compose (the api service listens on http://localhost:8080).
//
// Run with:
//     k6 run loadtest/api_smoke.js
//
// Auth: the API requires a valid "Authorization: Bearer <JWT>" header on all /v1/*
// routes (see internal/api/auth.go, internal/api/router.go). This script reads the
// token from the TASKFLOW_TOKEN environment variable rather than minting one itself,
// since minting requires the deployment's JWT_SECRET (see docker-compose.yml /
// k8s/secret.yaml), which this script has no business knowing. There is no CLI for
// this yet — internal/api/auth.go exposes a MintToken(secret, subject, ttl) helper
// that a real run would call from a one-off `go run` snippet or a small added CLI
// command (out of scope for this load test) to produce a JWT signed with the
// deployment's JWT_SECRET, then export it:
//
//     TASKFLOW_TOKEN=<jwt> k6 run loadtest/api_smoke.js
//
// Optionally override the target host with TASKFLOW_BASE_URL (defaults to
// http://localhost:8080, matching the "api" service's published port in
// docker-compose.yml).

import http from "k6/http";
import { check } from "k6";
import { Rate, Trend } from "k6/metrics";

const BASE_URL = __ENV.TASKFLOW_BASE_URL || "http://localhost:8080";
const TOKEN = __ENV.TASKFLOW_TOKEN;

if (!TOKEN) {
  throw new Error(
    "TASKFLOW_TOKEN env var must be set to a valid JWT for the deployment's JWT_SECRET " +
      "(see the comment at the top of this file for how to mint one)."
  );
}

// Custom metrics so thresholds can target this endpoint specifically rather than k6's
// blended http_req_duration/http_req_failed across every request type in the script.
const errorRate = new Rate("job_create_errors");
const jobCreateDuration = new Trend("job_create_duration", true);

export const options = {
  stages: [
    { duration: "30s", target: 20 }, // ramp up to 20 VUs
    { duration: "30s", target: 20 }, // hold at 20 VUs
    { duration: "15s", target: 0 }, // ramp down
  ],
  thresholds: {
    // 95% of POST /v1/jobs calls should complete in under 500ms.
    job_create_duration: ["p(95)<500"],
    // Fewer than 1% of requests should error (non-2xx or transport failure).
    job_create_errors: ["rate<0.01"],
  },
};

export default function () {
  const payload = JSON.stringify({
    name: "echo",
    payload: { hello: "world" },
    max_attempts: 3,
    timeout_seconds: 10,
  });

  const params = {
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${TOKEN}`,
    },
  };

  const res = http.post(`${BASE_URL}/v1/jobs`, payload, params);

  jobCreateDuration.add(res.timings.duration);

  const ok = check(res, {
    "status is 2xx": (r) => r.status >= 200 && r.status < 300,
  });
  errorRate.add(!ok);
}
