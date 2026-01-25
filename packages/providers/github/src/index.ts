import { AbstractProvider } from "@argus/core/abstractProvider";
import type { Connection } from "@argus/core/connection";
import type { EventEnvelope } from "@argus/core/event";
import type { TriggerDefinition } from "@argus/core/trigger";
import type { TransformInput } from "@argus/core/runtimeTypes";

export type GitHubConnectionAuth = {
	token?: string;
	webhookSecret?: string;
};

export class GitHubProvider extends AbstractProvider {
	name = "github";
	version = "0.1.0";

	constructor() {
		super();
		this.registerTrigger(issueCreatedTrigger());
		this.registerTrigger(issuesUpdatedTrigger());
	}

	validateConnection(connection: unknown): asserts connection is Connection {
		if (!connection || typeof connection !== "object") {
			throw new Error("Invalid connection");
		}
		const c = connection as Connection;
		if (!c.tenantId || !c.connectionId || !c.provider) {
			throw new Error("Connection missing required fields");
		}
		if (c.provider !== this.name) {
			throw new Error("Connection provider mismatch");
		}
	}
}

function issueCreatedTrigger(): TriggerDefinition {
	return {
		provider: "github",
		key: "issue.created",
		version: "1",
		mode: "webhook",
		async setup() {
			return {};
		},
		async teardown() {},
		async transform(input: TransformInput): Promise<EventEnvelope[]> {
			const payload = input.payload as GitHubIssueWebhook;
			if (!payload || payload.action !== "opened") return [];

			const issue = payload.issue;
			const repo = payload.repository;
			if (!issue || !repo) return [];

			const normalized = {
				repoFullName: repo.full_name,
				issueNumber: issue.number,
				title: issue.title,
				userLogin: issue.user?.login,
				url: issue.html_url,
			};

			const event: EventEnvelope = {
				id: "",
				type: "github.issue.created",
				occurredAt: issue.created_at ?? new Date().toISOString(),
				receivedAt: input.receivedAt,
				provider: input.provider,
				triggerKey: input.triggerKey,
				triggerVersion: input.triggerVersion,
				tenantId: input.connection.tenantId,
				connectionId: input.connection.connectionId,
				dedupeKey: "",
				data: {
					normalized,
					raw: payload,
				},
				meta: input.meta ?? {},
			};

			return [event];
		},
		dedupe(event: EventEnvelope): string {
			const headers = (event.meta?.headers ?? {}) as Record<string, string>;
			const delivery = headers["x-github-delivery"];
			if (delivery) return delivery;

			const raw = event.data?.raw as GitHubIssueWebhook | undefined;
			const issueId = raw?.issue?.id;
			const updatedAt = raw?.issue?.updated_at;
			if (issueId && updatedAt) return `${issueId}:${updatedAt}`;

			return `${event.connectionId}:${event.occurredAt}`;
		},
	};
}

type GitHubPollState = {
	since?: string;
};

type GitHubIssuePollPayload = {
	issue: GitHubIssueItem;
	repoFullName?: string;
};

function issuesUpdatedTrigger(): TriggerDefinition<unknown, GitHubPollState> {
	return {
		provider: "github",
		key: "issues.updated",
		version: "1",
		mode: "poll",
		async setup() {
			return {};
		},
		async teardown() {},
		async poll(ctx) {
			const auth = ctx.connection.auth as GitHubConnectionAuth | undefined;
			const token = auth?.token;
			const since =
				ctx.state?.since ??
				new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();

			const payloads = await fetchIssuesUpdatedSince({
				since,
				token,
				repoFullName: (
					ctx.connection.config as { repoFullName?: string } | undefined
				)?.repoFullName,
			});

			return { state: { since: new Date().toISOString() }, payloads };
		},
		async transform(input: TransformInput): Promise<EventEnvelope[]> {
			const payload = input.payload as GitHubIssuePollPayload;
			const issue = payload.issue;
			if (!issue || issue.pull_request) return [];

			const repoFullName =
				payload.repoFullName ?? parseRepoFullName(issue.repository_url);

			const normalized = {
				repoFullName,
				issueNumber: issue.number,
				title: issue.title,
				userLogin: issue.user?.login,
				url: issue.html_url,
			};

			const event: EventEnvelope = {
				id: "",
				type: "github.issue.updated",
				occurredAt: issue.updated_at ?? new Date().toISOString(),
				receivedAt: input.receivedAt,
				provider: input.provider,
				triggerKey: input.triggerKey,
				triggerVersion: input.triggerVersion,
				tenantId: input.connection.tenantId,
				connectionId: input.connection.connectionId,
				dedupeKey: "",
				data: {
					normalized,
					raw: payload,
				},
				meta: input.meta ?? {},
			};

			return [event];
		},
		dedupe(event: EventEnvelope): string {
			const raw = event.data?.raw as GitHubIssuePollPayload | undefined;
			const issueId = raw?.issue?.id;
			const updatedAt = raw?.issue?.updated_at;
			if (issueId && updatedAt) return `${issueId}:${updatedAt}`;
			return `${event.connectionId}:${event.occurredAt}`;
		},
	};
}

type GitHubIssueWebhook = {
	action?: string;
	issue?: {
		id: number;
		number: number;
		title: string;
		created_at?: string;
		updated_at?: string;
		html_url?: string;
		user?: { login?: string };
	};
	repository?: {
		full_name?: string;
	};
};

type GitHubIssueItem = {
	id: number;
	number: number;
	title: string;
	created_at?: string;
	updated_at?: string;
	html_url?: string;
	repository_url?: string;
	user?: { login?: string };
	pull_request?: unknown;
};

function parseRepoFullName(url?: string): string | undefined {
	if (!url) return undefined;
	const match = url.match(/\/repos\/([^/]+\/[^/]+)$/);
	return match?.[1];
}

async function fetchIssuesUpdatedSince(opts: {
	since: string;
	token?: string;
	repoFullName?: string;
}): Promise<GitHubIssuePollPayload[]> {
	const baseUrl = opts.repoFullName
		? `https://api.github.com/repos/${opts.repoFullName}/issues`
		: "https://api.github.com/issues";

	const params = new URLSearchParams();
	params.set("since", opts.since);
	params.set("state", "all");
	params.set("sort", "updated");
	params.set("direction", "asc");
	params.set("per_page", "100");

	const headers: Record<string, string> = {
		accept: "application/vnd.github+json",
		"user-agent": "argus",
	};
	if (opts.token) {
		headers.authorization = `Bearer ${opts.token}`;
	}

	const results: GitHubIssuePollPayload[] = [];
	let nextUrl: string | null = `${baseUrl}?${params.toString()}`;

	while (nextUrl) {
		const res = await fetch(nextUrl, { headers });
		if (!res.ok) {
			throw new Error(`GitHub poll failed: ${res.status}`);
		}
		const data = (await res.json()) as GitHubIssueItem[];
		for (const issue of data) {
			results.push({
				issue,
				repoFullName:
					opts.repoFullName ?? parseRepoFullName(issue.repository_url),
			});
		}

		const link = res.headers.get("link");
		nextUrl = link ? parseNextLink(link) : null;
	}

	return results;
}

function parseNextLink(linkHeader: string): string | null {
	const parts = linkHeader.split(",");
	for (const part of parts) {
		const match = part.match(/<([^>]+)>\s*;\s*rel="([^"]+)"/);
		if (!match) continue;
		const url = match[1];
		const rel = match[2];
		if (rel === "next") return url;
	}
	return null;
}
