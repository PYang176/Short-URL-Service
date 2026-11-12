import http from "k6/http";
import { check, fail } from "k6";

const baseURL = __ENV.BASE_URL || "http://127.0.0.1:18080";
const linkCount = Number(__ENV.LINK_COUNT || 100);
const rate = Number(__ENV.RATE || 100);
const duration = __ENV.DURATION || "30s";

export const options = {
  scenarios: {
    redirect: {
      executor: "constant-arrival-rate",
      rate,
      timeUnit: "1s",
      duration,
      preAllocatedVUs: Math.max(20, Math.ceil(rate / 10)),
      maxVUs: Math.max(100, rate * 2),
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<50", "p(99)<100"],
    checks: ["rate>0.99"],
  },
  discardResponseBodies: false,
};

export function setup() {
  const suffix = `${Date.now()}${Math.floor(Math.random() * 10000)}`;
  const email = `load-${suffix}@example.com`;
  const register = http.post(
    `${baseURL}/api/v1/auth/register`,
    JSON.stringify({
      email,
      password: "password123",
      display_name: `Load ${suffix}`,
    }),
    { headers: { "Content-Type": "application/json" }, tags: { name: "setup-register" } },
  );
  if (register.status !== 201) {
    fail(`register failed: ${register.status} ${register.body}`);
  }
  const auth = register.json();
  const headers = {
    Authorization: `Bearer ${auth.token.access_token}`,
    "Content-Type": "application/json",
  };

  const links = [];
  for (let index = 0; index < linkCount; index += 1) {
    const alias = `load${suffix}${index}`;
    const response = http.post(
      `${baseURL}/api/v1/links`,
      JSON.stringify({
        destination_url: `https://example.com/load/${index}`,
        alias,
        title: `Load ${index}`,
      }),
      {
        headers: {
          ...headers,
          "Idempotency-Key": `load-${suffix}-${index}`,
        },
        tags: { name: "setup-create" },
      },
    );
    if (response.status !== 201) {
      fail(`create link failed: ${response.status} ${response.body}`);
    }
    const created = response.json();
    links.push({ id: created.id, slug: created.slug });
  }

  // 首轮访问用于预热 Redis，不计入正式压测阈值。
  for (const link of links) {
    http.get(`${baseURL}/r/${link.slug}`, {
      redirects: 0,
      tags: { name: "setup-warmup" },
    });
  }

  return {
    links,
  };
}

export default function (data) {
  const link = data.links[Math.floor(Math.random() * data.links.length)];
  const response = http.get(`${baseURL}/r/${link.slug}`, {
    redirects: 0,
    tags: { name: "redirect" },
  });
  check(response, {
    "redirect returned 301/302": (result) => result.status === 301 || result.status === 302,
  });
}
