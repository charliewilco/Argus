import { defineConfig } from "tsdown";

export default defineConfig({
	entry: "src/runtime.ts",
	outDir: "dist",
	format: "esm",
	dts: true,
	fixedExtension: false,
});
