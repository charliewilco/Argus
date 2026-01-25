import { expect, test } from "bun:test";
import { GitHubProvider } from "./index";
import type { TriggerDefinition } from "@argus/core/trigger";

function getPollingTrigger(): TriggerDefinition {
	const provider = new GitHubProvider();
	const trigger = provider.getTriggers().find((t) => t.key === "issues.updated");
	if (!trigger) throw new Error("poll trigger missing");
	return trigger;
}

test("GitHub polling trigger paginates and returns payloads", async () => {
	const trigger = getPollingTrigger();

	const calls: string[] = [];
	const originalFetch = globalThis.fetch;
	globalThis.fetch = (async (input: RequestInfo | URL) => {
		const url = input.toString();
		calls.push(url);

		if (calls.length === 1) {
			const headers = new Headers({
				link:
					"<https://api.github.com/repos/octo/repo/issues?page=2&per_page=100>; rel=\"next\"",
			});
			return new Response(
				JSON.stringify([
					{
						id: 1,
						number: 10,
						title: "first",
						updated_at: "2024-01-01T00:00:00.000Z",
						repository_url: "https://api.github.com/repos/octo/repo",
					},
				]),
				{ headers },
			);
		}

		return new Response(
			JSON.stringify([
				{
					id: 2,
					number: 11,
					title: "second",
					updated_at: "2024-01-02T00:00:00.000Z",
					repository_url: "https://api.github.com/repos/octo/repo",
				},
			]),
		);
	}) as typeof fetch;

	try {
		const result = await trigger.poll?.({
			connection: {
				tenantId: "t1",
				connectionId: "c1",
				provider: "github",
				auth: { token: "token" },
				config: { repoFullName: "octo/repo" },
			},
			config: {},
			state: undefined,
			provider: "github",
			triggerKey: "issues.updated",
		});

		expect(calls.length).toBe(2);
		expect(calls[0]).toContain("https://api.github.com/repos/octo/repo/issues?");
		expect(calls[0]).toContain("since=");
		expect(calls[0]).toContain("per_page=100");
		expect(calls[1]).toBe(
			"https://api.github.com/repos/octo/repo/issues?page=2&per_page=100",
		);

		expect(result?.payloads?.length).toBe(2);
		expect(result?.payloads?.[0]?.repoFullName).toBe("octo/repo");
		expect(result?.state).toBeTruthy();
	} finally {
		globalThis.fetch = originalFetch;
	}
});
