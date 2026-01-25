import { verifyGitHubSignature } from "@argus/provider-github/verifyGitHubSignature";
import type { Runtime } from "@argus/runtime/runtime";

type WebhookRuntime = Pick<Runtime, "handleWebhook">;

export function createGitHubWebhookHandler(opts: {
	runtime: WebhookRuntime;
	webhookSecret?: string;
}) {
	return async function handleRequest(req: Request): Promise<Response> {
		const url = new URL(req.url);

		if (
			req.method === "POST" &&
			url.pathname === "/webhooks/github/issue.created"
		) {
			const rawBody = await req.text();
			const signature = req.headers.get("x-hub-signature-256");
			const secret = opts.webhookSecret;

			if (secret && !verifyGitHubSignature(secret, rawBody, signature)) {
				return new Response("invalid signature", { status: 401 });
			}

			const body = rawBody ? JSON.parse(rawBody) : {};
			const headers: Record<string, string> = {};
			for (const [key, value] of req.headers.entries()) {
				headers[key.toLowerCase()] = value;
			}

			const result = await opts.runtime.handleWebhook({
				provider: "github",
				triggerKey: "issue.created",
				body,
				headers,
				tenantId: "tenant_1",
				connectionId: "github_default",
			});

			return new Response(JSON.stringify(result), {
				status: result.accepted ? 202 : 400,
				headers: { "content-type": "application/json" },
			});
		}

		return new Response("not found", { status: 404 });
	};
}
