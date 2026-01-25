import { GitHubProvider } from "@argus/provider-github";
import { MemoryQueue } from "@argus/queue-memory";
import { Runtime } from "@argus/runtime/runtime";
import { MemoryEventStore } from "@argus/storage-memory";
import { createGitHubWebhookHandler } from "./handler";

const eventStore = new MemoryEventStore();
const queue = new MemoryQueue();
const runtime = new Runtime({ eventStore, queue });

const provider = new GitHubProvider();
runtime.registerProvider(provider);

runtime.registerConnection({
	tenantId: "tenant_1",
	connectionId: "github_default",
	provider: "github",
	auth: {
		token: process.env.GITHUB_TOKEN,
		webhookSecret: process.env.GITHUB_WEBHOOK_SECRET,
	},
	config: {},
});

runtime.onEvent(async (event) => {
	console.log("EVENT", JSON.stringify(event, null, 2));
	if (process.env.ARGUS_FAIL_HANDLER === "1") {
		throw new Error("forced handler failure");
	}
});

const server = Bun.serve({
	port: Number(process.env.PORT ?? 3000),
	fetch: createGitHubWebhookHandler({
		runtime,
		webhookSecret: process.env.GITHUB_WEBHOOK_SECRET,
	}),
});

console.log(`Argus example listening on http://localhost:${server.port}`);
