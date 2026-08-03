/**
 * Ainsel LLM Config extension for Pi.
 *
 * ⚠️ WORKAROUND — this extension exists only because pi-coding-agent 0.75.x
 * has no native way to set `temperature` (or any other sampling parameter)
 * from `models.json`. Verified by reading
 * `packages/coding-agent/src/core/model-registry.ts` upstream: the
 * `ModelDefinitionSchema` / `ModelOverrideSchema` / `ProviderConfigSchema`
 * TypeBox schemas enumerate every accepted field, and temperature is not
 * among them. The only documented mechanism is the
 * `before_provider_request` extension hook (see
 * `pi/examples/extensions/provider-payload.ts`).
 *
 * Once pi adds a native `temperature` slot to `models.json` (or an
 * equivalent provider-options block), DELETE this extension and let the
 * operator's models.json template emit the value in pi's native shape.
 * Tracking upstream: see pi project issue tracker.
 *
 * --- How it works ---
 *
 * The agent operator embeds `spec.llm.temperature` into the pi-models
 * ConfigMap under an `ainsel` sibling of `providers` at the root of
 * `models.json`. Pi ignores unknown root keys (TypeBox does not enforce
 * `additionalProperties: false` on the root schema), so the file
 * round-trips through pi's validator unchanged.
 *
 *     {
 *       "providers": { ... },
 *       "ainsel": {
 *         "temperature": 0.3
 *       }
 *     }
 *
 * This extension reads the same file pi reads, pulls `ainsel.temperature`
 * out, and registers a `before_provider_request` hook to inject it into
 * every payload. Keeps all LLM config in one ConfigMap; no extra env vars
 * on the pod.
 *
 * Unset / missing / malformed → provider defaults, no hook registered.
 * Invalid values are logged and ignored — never fail the agent at startup
 * over a typo in a sampling knob.
 */

import { readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const TAG = "pi-ainsel-llm-config";

interface AinselConfig {
	temperature?: unknown;
}

interface ModelsFile {
	ainsel?: AinselConfig;
}

function resolveModelsPath(): string {
	const overrideDir = process.env.PI_CODING_AGENT_DIR;
	const dir =
		overrideDir && overrideDir !== ""
			? overrideDir
			: join(homedir(), ".pi", "agent");
	return join(dir, "models.json");
}

function readAinselBlock(path: string): AinselConfig | undefined {
	let raw: string;
	try {
		raw = readFileSync(path, "utf8");
	} catch (err) {
		const code = (err as NodeJS.ErrnoException).code;
		if (code === "ENOENT") return undefined;
		console.error(`${TAG}: cannot read ${path}: ${err}`);
		return undefined;
	}
	let parsed: unknown;
	try {
		parsed = JSON.parse(raw);
	} catch (err) {
		console.error(`${TAG}: ${path} is not valid JSON: ${err}`);
		return undefined;
	}
	if (parsed === null || typeof parsed !== "object") return undefined;
	return (parsed as ModelsFile).ainsel;
}

function validateTemperature(raw: unknown): number | undefined {
	if (raw === undefined) return undefined;
	if (typeof raw !== "number" || !Number.isFinite(raw)) {
		console.error(
			`${TAG}: ainsel.temperature=${raw} is not a finite number; ignoring`,
		);
		return undefined;
	}
	if (raw < 0 || raw > 2) {
		console.error(
			`${TAG}: ainsel.temperature=${raw} outside [0,2]; ignoring`,
		);
		return undefined;
	}
	return raw;
}

export default function ainselLlmConfigExtension(pi: ExtensionAPI): void {
	const block = readAinselBlock(resolveModelsPath());
	const temperature = validateTemperature(block?.temperature);

	if (temperature === undefined) {
		console.error(`${TAG}: no overrides; provider defaults in effect`);
		return;
	}

	pi.on("before_provider_request", (event) => {
		return { ...event.payload, temperature };
	});

	console.error(`${TAG}: injecting temperature=${temperature} per request`);
}
