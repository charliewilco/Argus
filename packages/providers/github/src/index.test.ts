import { expect, test } from "bun:test";
import type { TriggerDefinition } from "@argus/core/trigger";
import { GitHubProvider } from "./index";

function getWebhookTrigger(): TriggerDefinition {
	const provider = new GitHubProvider();
	const trigger = provider.getTriggers().find((t) => t.key === "issue.created");
	if (!trigger) throw new Error("webhook trigger missing");
	return trigger;
}

function getPollingTrigger(): TriggerDefinition {
	const provider = new GitHubProvider();
	const trigger = provider
		.getTriggers()
		.find((t) => t.key === "issues.updated");
	if (!trigger) throw new Error("poll trigger missing");
	return trigger;
}

test("GitHub issue.created transform normalizes and dedupes", async () => {
	const trigger = getWebhookTrigger();
	const payload = {
		action: "opened",
		issue: {
			id: 123,
			number: 45,
			title: "Bug",
			created_at: "2024-01-01T00:00:00.000Z",
			updated_at: "2024-01-02T00:00:00.000Z",
			html_url: "https://github.com/octo/repo/issues/45",
			user: { login: "octo" },
		},
		repository: { full_name: "octo/repo" },
	};

	const [event] = await trigger.transform({
		provider: "github",
		triggerKey: "issue.created",
		triggerVersion: "1",
		connection: {
			tenantId: "t1",
			connectionId: "c1",
			provider: "github",
			auth: {},
		},
		receivedAt: "2024-01-03T00:00:00.000Z",
		payload,
		meta: { headers: { "x-github-delivery": "delivery-1" } },
	});

	expect(event?.type).toBe("github.issue.created");
	expect(event?.data?.normalized).toEqual({
		repoFullName: "octo/repo",
		issueNumber: 45,
		title: "Bug",
		userLogin: "octo",
		url: "https://github.com/octo/repo/issues/45",
	});

	// biome-ignore lint/style/noNonNullAssertion: Test
	const dedupe = trigger.dedupe(event!);
	expect(dedupe).toBe("delivery-1");
});

test("GitHub issue.created ignores non-opened actions", async () => {
	const trigger = getWebhookTrigger();
	const events = await trigger.transform({
		provider: "github",
		triggerKey: "issue.created",
		triggerVersion: "1",
		connection: {
			tenantId: "t1",
			connectionId: "c1",
			provider: "github",
			auth: {},
		},
		receivedAt: "2024-01-03T00:00:00.000Z",
		payload: { action: "edited" },
	});

	expect(events.length).toBe(0);
});

test("GitHub issues.updated skips pull requests and parses repo name", async () => {
	const trigger = getPollingTrigger();
	const events = await trigger.transform({
		provider: "github",
		triggerKey: "issues.updated",
		triggerVersion: "1",
		connection: {
			tenantId: "t1",
			connectionId: "c1",
			provider: "github",
			auth: {},
		},
		receivedAt: "2024-01-03T00:00:00.000Z",
		payload: {
			issue: {
				id: 456,
				number: 99,
				title: "Update",
				updated_at: "2024-01-03T00:00:00.000Z",
				html_url: "https://github.com/octo/repo/issues/99",
				repository_url: "https://api.github.com/repos/octo/repo",
				user: { login: "octo" },
			},
		},
	});

	expect(events.length).toBe(1);
	expect(events[0]?.data?.normalized).toEqual({
		repoFullName: "octo/repo",
		issueNumber: 99,
		title: "Update",
		userLogin: "octo",
		url: "https://github.com/octo/repo/issues/99",
	});

	const skipped = await trigger.transform({
		provider: "github",
		triggerKey: "issues.updated",
		triggerVersion: "1",
		connection: {
			tenantId: "t1",
			connectionId: "c1",
			provider: "github",
			auth: {},
		},
		receivedAt: "2024-01-03T00:00:00.000Z",
		payload: {
			issue: {
				id: 789,
				number: 100,
				title: "PR",
				updated_at: "2024-01-03T00:00:00.000Z",
				pull_request: {},
			},
		},
	});

	expect(skipped.length).toBe(0);
});

test("GitHub polling trigger surfaces fetch errors", async () => {
	const trigger = getPollingTrigger();

	const originalFetch = globalThis.fetch;
	globalThis.fetch = (async () =>
		new Response("nope", { status: 403 })) as unknown as typeof fetch;

	try {
		await expect(
			trigger.poll?.({
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
			}),
		).rejects.toThrow("GitHub poll failed: 403");
	} finally {
		globalThis.fetch = originalFetch;
	}
});

test("GitHub polling trigger paginates and returns payloads", async () => {
	const trigger = getPollingTrigger();

	const calls: string[] = [];
	const originalFetch = globalThis.fetch;
	globalThis.fetch = (async (input: RequestInfo | URL) => {
		const url = input.toString();
		calls.push(url);

		if (calls.length === 1) {
			const headers = new Headers({
				link: '<https://api.github.com/repos/octo/repo/issues?page=2&per_page=100>; rel="next"',
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
		expect(calls[0]).toContain(
			"https://api.github.com/repos/octo/repo/issues?",
		);
		expect(calls[0]).toContain("since=");
		expect(calls[0]).toContain("per_page=100");
		expect(calls[1]).toBe(
			"https://api.github.com/repos/octo/repo/issues?page=2&per_page=100",
		);

		expect(result?.payloads?.length).toBe(2);
		// @ts-expect-error this is just an assertion test
		expect(result?.payloads?.[0]?.repoFullName).toBe("octo/repo");
		expect(result?.state).toBeTruthy();
	} finally {
		globalThis.fetch = originalFetch;
	}
});
