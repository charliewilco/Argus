import { expect, test } from "bun:test";
import { createHmac } from "node:crypto";
import { createGitHubWebhookHandler } from "./handler";

test("example app workspace is wired for tests", () => {
	expect(true).toBe(true);
});

test("webhook handler passes payload to runtime and returns 202", async () => {
	const calls: Array<unknown> = [];
	const handler = createGitHubWebhookHandler({
		runtime: {
			handleWebhook: async (input) => {
				calls.push(input);
				return { accepted: true };
			},
		},
	});

	const body = JSON.stringify({ action: "opened" });
	const req = new Request("http://localhost/webhooks/github/issue.created", {
		method: "POST",
		body,
		headers: {
			"content-type": "application/json",
			"X-Test": "1",
		},
	});

	const res = await handler(req);
	expect(res.status).toBe(202);
	expect(calls.length).toBe(1);
	const call = calls[0] as { headers: Record<string, string>; body: unknown };
	expect(call.headers["x-test"]).toBe("1");
	expect(call.body).toEqual({ action: "opened" });
});

test("webhook handler rejects invalid signatures", async () => {
	const handler = createGitHubWebhookHandler({
		runtime: {
			handleWebhook: async () => ({ accepted: true }),
		},
		webhookSecret: "secret",
	});

	const req = new Request("http://localhost/webhooks/github/issue.created", {
		method: "POST",
		body: JSON.stringify({ hello: "world" }),
		headers: { "x-hub-signature-256": "sha256=bad" },
	});

	const res = await handler(req);
	expect(res.status).toBe(401);
});

test("webhook handler accepts valid signatures", async () => {
	const secret = "secret";
	const body = JSON.stringify({ hello: "world" });
	const hmac = createHmac("sha256", secret);
	hmac.update(body);
	const signature = `sha256=${hmac.digest("hex")}`;

	const handler = createGitHubWebhookHandler({
		runtime: {
			handleWebhook: async () => ({ accepted: true }),
		},
		webhookSecret: secret,
	});

	const req = new Request("http://localhost/webhooks/github/issue.created", {
		method: "POST",
		body,
		headers: { "x-hub-signature-256": signature },
	});

	const res = await handler(req);
	expect(res.status).toBe(202);
});

test("webhook handler returns 404 for unknown routes", async () => {
	const handler = createGitHubWebhookHandler({
		runtime: {
			handleWebhook: async () => ({ accepted: true }),
		},
	});

	const res = await handler(
		new Request("http://localhost/unknown", { method: "GET" }),
	);
	expect(res.status).toBe(404);
});
