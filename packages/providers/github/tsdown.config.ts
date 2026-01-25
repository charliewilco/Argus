import { defineConfig } from "tsdown";

export default defineConfig({
	entry: ["src/index.ts", "src/verifyGitHubSignature.ts"],
	outDir: "dist",
	format: "esm",
	dts: true,
	fixedExtension: false,
});
