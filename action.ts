import { chmodSync, createWriteStream, existsSync, mkdtempSync } from "node:fs";
import { mkdir, rm } from "node:fs/promises";
import { get } from "node:https";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync, spawnSync } from "node:child_process";
import { pipeline } from "node:stream/promises";

type Inputs = {
  accessToken: string;
  outputDir: string;
  version: string;
  releaseRepository: string;
  excludeRepos?: string;
  excludeLangs?: string;
  excludePrivate?: string;
  includeContributedRepositories?: string;
  excludeForkedRepos?: string;
  maxRetries?: string;
};

async function main(): Promise<void> {
  const inputs = readInputs();
  const platform = mapPlatform(process.platform);
  const arch = mapArch(process.arch);
  const extension = platform === "windows" ? "zip" : "tar.gz";
  const assetName = `github-stats-generator_${platform}_${arch}.${extension}`;
  const normalizedVersion = normalizeVersion(inputs.version);
  const baseURL =
    normalizedVersion === "latest"
      ? `https://github.com/${inputs.releaseRepository}/releases/latest/download`
      : `https://github.com/${inputs.releaseRepository}/releases/download/${normalizedVersion}`;
  const downloadURL = `${baseURL}/${assetName}`;

  console.log(`github-stats-generator action: downloading ${assetName} from ${downloadURL}`);
  const workspace = mkdtempSync(join(tmpdir(), "github-stats-generator-action-"));
  const archivePath = join(workspace, assetName);
  const extractDir = join(workspace, "extract");
  await mkdir(extractDir, { recursive: true });

  try {
    await downloadFile(downloadURL, archivePath);
    extractArchive(archivePath, extractDir, platform);

    const binaryName = platform === "windows" ? "github-stats-generator.exe" : "github-stats-generator";
    const binaryPath = join(extractDir, binaryName);
    if (!existsSync(binaryPath)) {
      throw new Error(`expected binary at ${binaryPath}, but it was not found after extraction`);
    }
    if (platform !== "windows") {
      chmodSync(binaryPath, 0o755);
    }

    console.log(`github-stats-generator action: generating SVGs into ${inputs.outputDir}`);
    const env = {
      ...process.env,
      ACCESS_TOKEN: inputs.accessToken,
      OUTPUT_DIR: inputs.outputDir,
    } as NodeJS.ProcessEnv;

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

    const result = spawnSync(binaryPath, [], {
      env,
      stdio: "inherit",
    });
    if (result.status !== 0) {
      throw new Error(`github-stats-generator exited with status ${result.status ?? "unknown"}`);
    }
    console.log("github-stats-generator action: done.");
  } finally {
    await rm(workspace, { recursive: true, force: true });
  }
}

function readInputs(): Inputs {
  const accessToken = input("access_token", true);
  const releaseRepository =
    input("release_repository", false) || process.env.GITHUB_ACTION_REPOSITORY || "";
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

function input(name: string, required: boolean): string {
  const key = `INPUT_${name.toUpperCase()}`;
  const value = (process.env[key] || "").trim();
  if (!value && required) {
    throw new Error(`missing required input: ${name}`);
  }
  return value;
}

function normalizeVersion(version: string): string {
  const trimmed = version.trim();
  if (trimmed === "" || trimmed === "latest") {
    return "latest";
  }
  if (trimmed.startsWith("v")) {
    return trimmed;
  }
  return `v${trimmed}`;
}

function mapPlatform(platform: NodeJS.Platform): "linux" | "darwin" | "windows" {
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

function mapArch(arch: string): "amd64" | "arm64" {
  switch (arch) {
    case "x64":
      return "amd64";
    case "arm64":
      return "arm64";
    default:
      throw new Error(`unsupported runner architecture: ${arch}`);
  }
}

async function downloadFile(url: string, destination: string): Promise<void> {
  const response = await request(url);
  if (response.statusCode < 200 || response.statusCode >= 300 || !response.stream) {
    throw new Error(`download failed with status ${response.statusCode} for ${url}`);
  }
  await pipeline(response.stream, createWriteStream(destination));
}

function request(
  url: string,
  redirectsLeft = 5,
): Promise<{ statusCode: number; stream?: NodeJS.ReadableStream }> {
  return new Promise((resolve, reject) => {
    const req = get(
      url,
      {
        headers: {
          "User-Agent": "github-stats-generator-action",
          Accept: "application/octet-stream",
        },
      },
      (res) => {
        const statusCode = res.statusCode ?? 0;
        const location = res.headers.location;
        if (
          statusCode >= 300 &&
          statusCode < 400 &&
          location &&
          redirectsLeft > 0
        ) {
          res.resume();
          resolve(request(location, redirectsLeft - 1));
          return;
        }
        resolve({ statusCode, stream: res });
      },
    );
    req.on("error", reject);
  });
}

function extractArchive(
  archivePath: string,
  extractDir: string,
  platform: "linux" | "darwin" | "windows",
): void {
  if (platform === "windows") {
    execFileSync(
      "powershell",
      [
        "-NoProfile",
        "-Command",
        `Expand-Archive -Path '${archivePath}' -DestinationPath '${extractDir}' -Force`,
      ],
      { stdio: "inherit" },
    );
    return;
  }

  execFileSync("tar", ["-xzf", archivePath, "-C", extractDir], {
    stdio: "inherit",
  });
}

main().catch((error) => {
  const message = error instanceof Error ? error.message : String(error);
  console.error(`::error::${message}`);
  process.exit(1);
});
