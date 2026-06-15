"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
const node_fs_1 = require("node:fs");
const promises_1 = require("node:fs/promises");
const node_https_1 = require("node:https");
const node_os_1 = require("node:os");
const node_path_1 = require("node:path");
const node_child_process_1 = require("node:child_process");
const promises_2 = require("node:stream/promises");
async function main() {
    const inputs = readInputs();
    const platform = mapPlatform(process.platform);
    const arch = mapArch(process.arch);
    const extension = platform === "windows" ? "zip" : "tar.gz";
    const assetName = `github-stats-generator_${platform}_${arch}.${extension}`;
    const normalizedVersion = normalizeVersion(inputs.version);
    const baseURL = normalizedVersion === "latest"
        ? `https://github.com/${inputs.releaseRepository}/releases/latest/download`
        : `https://github.com/${inputs.releaseRepository}/releases/download/${normalizedVersion}`;
    const downloadURL = `${baseURL}/${assetName}`;
    console.log(`github-stats-generator action: downloading ${assetName} from ${downloadURL}`);
    const workspace = (0, node_fs_1.mkdtempSync)((0, node_path_1.join)((0, node_os_1.tmpdir)(), "github-stats-generator-action-"));
    const archivePath = (0, node_path_1.join)(workspace, assetName);
    const extractDir = (0, node_path_1.join)(workspace, "extract");
    await (0, promises_1.mkdir)(extractDir, { recursive: true });
    try {
        await downloadFile(downloadURL, archivePath);
        extractArchive(archivePath, extractDir, platform);
        const binaryName = platform === "windows" ? "github-stats-generator.exe" : "github-stats-generator";
        const binaryPath = (0, node_path_1.join)(extractDir, binaryName);
        if (!(0, node_fs_1.existsSync)(binaryPath)) {
            throw new Error(`expected binary at ${binaryPath}, but it was not found after extraction`);
        }
        if (platform !== "windows") {
            (0, node_fs_1.chmodSync)(binaryPath, 0o755);
        }
        console.log(`github-stats-generator action: generating SVGs into ${inputs.outputDir}`);
        const env = {
            ...process.env,
            ACCESS_TOKEN: inputs.accessToken,
            OUTPUT_DIR: inputs.outputDir,
        };
        if (inputs.excludeRepos) {
            env.EXCLUDE_REPOS = inputs.excludeRepos;
        }
        if (inputs.excludeLangs) {
            env.EXCLUDE_LANGS = inputs.excludeLangs;
        }
        if (inputs.excludePrivate) {
            env.EXCLUDE_PRIVATE = inputs.excludePrivate;
        }
        if (inputs.includeContributedRepositories) {
            env.INCLUDE_CONTRIBUTED_REPOSITORIES = inputs.includeContributedRepositories;
        }
        if (inputs.excludeForkedRepos) {
            env.EXCLUDE_FORKED_REPOS = inputs.excludeForkedRepos;
        }
        if (inputs.maxRetries) {
            env.MAX_RETRIES = inputs.maxRetries;
        }
        const result = (0, node_child_process_1.spawnSync)(binaryPath, [], {
            env,
            stdio: "inherit",
        });
        if (result.status !== 0) {
            throw new Error(`github-stats-generator exited with status ${result.status ?? "unknown"}`);
        }
        console.log("github-stats-generator action: done.");
    }
    finally {
        await (0, promises_1.rm)(workspace, { recursive: true, force: true });
    }
}
function readInputs() {
    const accessToken = input("access_token", true);
    const releaseRepository = input("release_repository", false) || process.env.GITHUB_ACTION_REPOSITORY || "";
    if (!releaseRepository) {
        throw new Error("release_repository input is required when GITHUB_ACTION_REPOSITORY is unavailable");
    }
    return {
        accessToken,
        outputDir: input("output_dir", false) || ".",
        version: input("version", false) || "latest",
        releaseRepository,
        excludeRepos: input("exclude_repos", false),
        excludeLangs: input("exclude_langs", false),
        excludePrivate: input("exclude_private", false),
        includeContributedRepositories: input("include_contributed_repositories", false),
        excludeForkedRepos: input("exclude_forked_repos", false),
        maxRetries: input("max_retries", false),
    };
}
function input(name, required) {
    const key = `INPUT_${name.toUpperCase()}`;
    const value = (process.env[key] || "").trim();
    if (!value && required) {
        throw new Error(`missing required input: ${name}`);
    }
    return value;
}
function normalizeVersion(version) {
    const trimmed = version.trim();
    if (trimmed === "" || trimmed === "latest") {
        return "latest";
    }
    if (trimmed.startsWith("v")) {
        return trimmed;
    }
    return `v${trimmed}`;
}
function mapPlatform(platform) {
    switch (platform) {
        case "linux":
            return "linux";
        case "darwin":
            return "darwin";
        case "win32":
            return "windows";
        default:
            throw new Error(`unsupported runner platform: ${platform}`);
    }
}
function mapArch(arch) {
    switch (arch) {
        case "x64":
            return "amd64";
        case "arm64":
            return "arm64";
        default:
            throw new Error(`unsupported runner architecture: ${arch}`);
    }
}
async function downloadFile(url, destination) {
    const response = await request(url);
    if (response.statusCode < 200 || response.statusCode >= 300 || !response.stream) {
        throw new Error(`download failed with status ${response.statusCode} for ${url}`);
    }
    await (0, promises_2.pipeline)(response.stream, (0, node_fs_1.createWriteStream)(destination));
}
function request(url, redirectsLeft = 5) {
    return new Promise((resolve, reject) => {
        const req = (0, node_https_1.get)(url, {
            headers: {
                "User-Agent": "github-stats-generator-action",
                Accept: "application/octet-stream",
            },
        }, (res) => {
            const statusCode = res.statusCode ?? 0;
            const location = res.headers.location;
            if (statusCode >= 300 &&
                statusCode < 400 &&
                location &&
                redirectsLeft > 0) {
                res.resume();
                resolve(request(location, redirectsLeft - 1));
                return;
            }
            resolve({ statusCode, stream: res });
        });
        req.on("error", reject);
    });
}
function extractArchive(archivePath, extractDir, platform) {
    if (platform === "windows") {
        (0, node_child_process_1.execFileSync)("powershell", [
            "-NoProfile",
            "-Command",
            `Expand-Archive -Path '${archivePath}' -DestinationPath '${extractDir}' -Force`,
        ], { stdio: "inherit" });
        return;
    }
    (0, node_child_process_1.execFileSync)("tar", ["-xzf", archivePath, "-C", extractDir], {
        stdio: "inherit",
    });
}
main().catch((error) => {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`::error::${message}`);
    process.exit(1);
});
