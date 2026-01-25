import { defineConfig } from "tsdown";

export default defineConfig({
	entry: [
		"src/event.ts",
		"src/trigger.ts",
		"src/provider.ts",
		"src/abstractProvider.ts",
		"src/connection.ts",
		"src/id.ts",
		"src/eventStore.ts",
		"src/queue.ts",
		"src/runtimeTypes.ts",
	],
	outDir: "dist",
	format: "esm",
	dts: true,
	fixedExtension: false,
});
