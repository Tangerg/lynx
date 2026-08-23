import { useInfiniteQuery } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";

import type {
	Item,
	RunSummary,
	RuntimeConnection,
} from "@lyra/runtime-contract";

import {
	listItems,
	runtimeQueryKeys,
} from "../../runtime/runtimeQueries";
import { useLocalization } from "../localization/Localization";

const historyPageSize = 100;

export interface SessionHistoryView {
	items: Item[];
	runs: RunSummary[];
	hasOlder: boolean;
	loading: boolean;
	error?: string;
	loadOlder(): Promise<void>;
}

export function useSessionHistory(
	connection: RuntimeConnection,
	sessionId: string | undefined,
	mountedItems: Item[],
): SessionHistoryView {
	const { t } = useLocalization();
	const query = useInfiniteQuery({
		queryKey: runtimeQueryKeys.sessionHistory(
			connection,
			sessionId ?? "unselected",
		),
		queryFn: ({ pageParam, signal }) =>
			listItems(
				connection,
				{
					scope: { type: "session", sessionId: sessionId ?? "" },
					order: "desc",
					limit: historyPageSize,
					...(pageParam === undefined ? {} : { cursor: pageParam }),
				},
				signal,
			),
		initialPageParam: undefined as string | undefined,
		getNextPageParam: (page) => page.nextCursor,
		enabled: sessionId !== undefined,
		retry: 2,
	});

	const mountedIDs = useMemo(
		() => new Set(mountedItems.map((item) => item.id)),
		[mountedItems],
	);
	const material = useMemo(() => {
		const pages = query.data?.pages ?? [];
		const newestFirst = pages.flatMap((page) => page.data);
		const items = newestFirst
			.toReversed()
			.filter((item) => !mountedIDs.has(item.id));
		const runByID = new Map<string, RunSummary>();
		for (const page of pages.toReversed()) {
			for (const run of page.runs) runByID.set(run.id, run);
		}
		return { items, runs: [...runByID.values()] };
	}, [mountedIDs, query.data?.pages]);

	const loadOlder = useCallback(async () => {
		if (query.isError) {
			await query.refetch();
			return;
		}
		if (!query.hasNextPage || query.isFetchingNextPage) return;
		const visibleBefore = material.items.length;
		let result = await query.fetchNextPage();
		while (
			result.hasNextPage &&
			!result.isFetchingNextPage &&
			countOlderItems(result.data?.pages ?? [], mountedIDs) === visibleBefore
		) {
			result = await query.fetchNextPage();
		}
	}, [material.items.length, mountedIDs, query]);

	return {
		items: material.items,
		runs: material.runs,
		hasOlder: query.hasNextPage === true,
		loading: query.isPending || query.isFetchingNextPage,
		error: query.isError ? messageOf(query.error, t("narrative.olderHistoryFailed")) : undefined,
		loadOlder,
	};
}

function countOlderItems(
	pages: Array<{ data: Item[] }>,
	mountedIDs: Set<string>,
): number {
	const ids = new Set<string>();
	for (const page of pages) {
		for (const item of page.data) {
			if (!mountedIDs.has(item.id)) ids.add(item.id);
		}
	}
	return ids.size;
}

function messageOf(error: unknown, fallback: string): string {
	return error instanceof Error ? error.message : fallback;
}
